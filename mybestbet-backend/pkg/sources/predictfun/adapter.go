package predictfun

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name               = "predictfun"
	apiKeyHeader       = "x-api-key"
	sportsMatchVariant = "SPORTS_MATCH"
	categoriesPageSize = 100
	maxCategoryPages   = 20
	settlementWindow   = 2 * time.Hour
	minPollInterval    = 5 * time.Second
)

var matchVariants = []string{
	sportsMatchVariant, "SPORTS_TEAM_MATCH", "SPORTS_NBA", "SPORTS_FIFA_WORLD_CUP", "SPORTS_FIFA_FRIENDLIES",
	"ESPORTS_LOL", "ESPORTS_CS2", "ESPORTS_DOTA2",
}

type Adapter struct {
	deps    sources.Deps
	leagues leagueTable
	catalog catalog
}

func init() { sources.Register(Name, New) }

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{deps: deps, leagues: newLeagueTable(deps.Config.LeagueIDs)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     supportedMarkets,
		Leagues:     a.leagues.codes(),
		Push:        a.deps.Config.APIKey != "",
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	requested := make(map[string]bool, len(leagues))
	for _, league := range leagues {
		requested[league] = true
	}
	categories, errs := a.openVariantCategories(ctx, matchVariants)
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	snap.Errors = errs
	for _, c := range categories {
		if ev, ok := a.sourceEvent(c); ok && requested[ev.LeagueHint] {
			snap.Events = append(snap.Events, ev)
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) sourceEvent(c category) (canon.SourceEvent, bool) {
	league, mapped := a.leagues.leagueOf(c)
	if !mapped {
		return canon.SourceEvent{}, false
	}
	sport, _ := a.leagues.sportOf(c.Slug)
	teams, hasTeams := teamsOf(c, sport)
	if !hasTeams {
		return canon.SourceEvent{}, false
	}
	return canon.SourceEvent{
		SourceID:   c.Slug,
		LeagueHint: league,
		Home:       teams.home.Name,
		Away:       teams.away.Name,
		Kickoff:    kickoffOf(c),
	}, true
}

func kickoffOf(c category) time.Time {
	opensTradingBeforeKickoff := c.MarketVariant == sportsMatchVariant
	if opensTradingBeforeKickoff && c.EndsAt != nil && !c.EndsAt.IsZero() {
		return c.EndsAt.Add(-settlementWindow).UTC()
	}
	return c.StartsAt.UTC()
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	b, err := a.loadBoard(ctx, eventIDs)
	if err != nil {
		return canon.Snapshot{}, err
	}
	return b.snapshot(), nil
}

func (a *Adapter) openVariantCategories(ctx context.Context, variants []string) ([]category, []error) {
	var categories []category
	var errs []error
	for _, variant := range variants {
		listed, err := a.openCategories(ctx, variant)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		categories = append(categories, listed...)
	}
	return categories, errs
}

func (a *Adapter) openCategories(ctx context.Context, variant string) ([]category, error) {
	query := url.Values{
		"status":        {"OPEN"},
		"marketVariant": {variant},
		"first":         {strconv.Itoa(categoriesPageSize)},
	}
	var categories []category
	for range maxCategoryPages {
		var page categoryList
		if err := a.getJSON(ctx, "/v1/categories?"+query.Encode(), &page); err != nil {
			return nil, fmt.Errorf("list %s categories: %w", variant, err)
		}
		categories = append(categories, page.Data...)
		if page.Cursor == "" || len(page.Data) < categoriesPageSize {
			break
		}
		query.Set("after", page.Cursor)
	}
	return categories, nil
}

func (a *Adapter) loadBoard(ctx context.Context, eventIDs []string) (*board, error) {
	now := a.deps.Clock()
	open, errs := a.openCatalog(ctx)
	var markets []pricedMarket
	quotes := map[int64]quote{}
	for _, id := range eventIDs {
		if match := a.matchMarkets(open, id); match != nil {
			markets = append(markets, match.markets...)
			maps.Copy(quotes, match.quotes)
		}
	}
	b := newBoard(markets, now)
	for _, marketID := range b.marketIDs() {
		if q, inline := quotes[marketID]; inline {
			b.setQuote(marketID, q, now)
			continue
		}
		var env orderbookEnvelope
		if err := a.getJSON(ctx, "/v1/markets/"+strconv.FormatInt(marketID, 10)+"/orderbook", &env); err != nil {
			errs = append(errs, fmt.Errorf("orderbook %d: %w", marketID, err))
			continue
		}
		b.setQuote(marketID, quoteOf(env.Data), now)
	}
	b.errs = errs
	if len(markets) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return b, nil
}

func (a *Adapter) matchMarkets(open map[string]category, matchSlug string) *matchBuilder {
	sport, known := a.leagues.sportOf(matchSlug)
	if !known {
		a.deps.Log().Warn("match slug has no known league prefix", "source", Name, "event_id", matchSlug)
		return nil
	}
	var builder *matchBuilder
	for _, suffix := range categorySuffixesFor(sport) {
		c, found := open[matchSlug+suffix]
		if !found {
			continue
		}
		if builder == nil {
			teams, ok := teamsOf(c, sport)
			if !ok {
				a.deps.Log().Warn("match category has no home and away teams", "source", Name, "event_id", matchSlug)
				return nil
			}
			builder = newMatchBuilder(matchSlug, sport, teams, func(id string) { a.deps.DropUnknownMarket(Name, id) })
		}
		builder.addCategory(c)
	}
	return builder
}

func (a *Adapter) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.deps.Config.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	if key := a.deps.Config.APIKey; key != "" {
		req.Header.Set(apiKeyHeader, key)
	}
	body, err := a.deps.Do(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
