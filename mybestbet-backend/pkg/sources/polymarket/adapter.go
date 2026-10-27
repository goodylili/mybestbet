package polymarket

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name              = "polymarket"
	defaultCLOBURL    = "https://clob.polymarket.com"
	discoveryPageSize = 100
	maxDiscoveryPages = 20
	discoveryHorizon  = 90 * 24 * time.Hour
	booksPerRequest   = 200
	minPollInterval   = 2 * time.Second
)

var baseMatchSlug = regexp.MustCompile(`^[a-z0-9]+-[a-z0-9]+-[a-z0-9]+-\d{4}-\d{2}-\d{2}$`)

type Adapter struct {
	deps    sources.Deps
	leagues map[string]venueLeague
	sports  map[string]canon.Sport
}

func init() { sources.Register(Name, New) }

func New(deps sources.Deps) sources.Adapter {
	leagues := leaguesWithOverrides(deps.Config.LeagueIDs)
	return &Adapter{deps: deps, leagues: leagues, sports: sportsBySportCode(leagues)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     supportedMarkets,
		Leagues:     slices.Sorted(maps.Keys(a.leagues)),
		Push:        true,
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	filters, err := a.sportFilters(ctx)
	if err != nil {
		return canon.Snapshot{}, err
	}
	now := a.deps.Clock()
	snap := sources.NewSnapshot(Name, now)
	seen := map[string]bool{}
	eventsBySport := map[string][]gammaEvent{}
	for _, code := range leagues {
		league, supported := a.leagues[code]
		filter, listed := filters[league.sportCode]
		if !supported || !listed {
			continue
		}
		events, fetched := eventsBySport[league.sportCode]
		if !fetched {
			if events, err = a.sportEvents(ctx, filter, now); err != nil {
				snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", code, err))
			}
			eventsBySport[league.sportCode] = events
		}
		for _, ev := range events {
			se, isMatch := sourceEvent(ev, code)
			if isMatch && league.lists(ev) && !seen[se.SourceID] {
				seen[se.SourceID] = true
				snap.Events = append(snap.Events, se)
			}
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	b, err := a.loadBoard(ctx, eventIDs)
	if err != nil {
		return canon.Snapshot{}, err
	}
	return b.snapshot(), nil
}

type eventFilter struct {
	param string
	value string
}

func (a *Adapter) sportFilters(ctx context.Context) (map[string]eventFilter, error) {
	var sports []gammaSport
	if err := a.deps.GetJSON(ctx, a.deps.Config.BaseURL+"/sports", &sports); err != nil {
		return nil, fmt.Errorf("list sports: %w", err)
	}
	filters := make(map[string]eventFilter, len(sports))
	for _, s := range sports {
		filters[s.Sport] = s.eventFilter()
	}
	return filters, nil
}

func (s gammaSport) eventFilter() eventFilter {
	if series, _, _ := strings.Cut(s.Series, ","); series != "" {
		return eventFilter{param: "series_id", value: series}
	}
	return eventFilter{param: "tag_id", value: strconv.FormatInt(s.PrimaryTagID, 10)}
}

func (a *Adapter) sportEvents(ctx context.Context, filter eventFilter, now time.Time) ([]gammaEvent, error) {
	query := url.Values{
		filter.param:   {filter.value},
		"active":       {"true"},
		"closed":       {"false"},
		"limit":        {strconv.Itoa(discoveryPageSize)},
		"end_date_min": {now.Format(time.RFC3339)},
		"end_date_max": {now.Add(discoveryHorizon).Format(time.RFC3339)},
	}
	var events []gammaEvent
	for page := range maxDiscoveryPages {
		query.Set("offset", strconv.Itoa(page*discoveryPageSize))
		var batch []gammaEvent
		if err := a.deps.GetJSON(ctx, a.deps.Config.BaseURL+"/events?"+query.Encode(), &batch); err != nil {
			return nil, fmt.Errorf("list events for %s %s: %w", filter.param, filter.value, err)
		}
		events = append(events, batch...)
		if len(batch) < discoveryPageSize {
			break
		}
	}
	return events, nil
}

func sourceEvent(ev gammaEvent, league string) (canon.SourceEvent, bool) {
	if !baseMatchSlug.MatchString(ev.Slug) || ev.StartTime.IsZero() {
		return canon.SourceEvent{}, false
	}
	teams, ok := teamsOf([]gammaEvent{ev})
	if !ok {
		return canon.SourceEvent{}, false
	}
	return canon.SourceEvent{
		SourceID:   ev.Slug,
		LeagueHint: league,
		Home:       teams.home.Name,
		Away:       teams.away.Name,
		Kickoff:    ev.StartTime.UTC(),
	}, true
}

func (a *Adapter) loadBoard(ctx context.Context, eventIDs []string) (*board, error) {
	now := a.deps.Clock()
	var markets []pricedMarket
	var errs []error
	for _, id := range eventIDs {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		matchMarkets, err := a.matchMarkets(ctx, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		markets = append(markets, matchMarkets...)
	}
	books, unbooked, err := a.fetchBooks(ctx, newBoard(markets, now).tokenIDs())
	if err != nil {
		errs = append(errs, err)
	}
	markets = slices.DeleteFunc(markets, func(pm pricedMarket) bool {
		return slices.ContainsFunc(pm.tokens, func(token string) bool { return unbooked[token] })
	})
	if len(markets) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	b := newBoard(markets, now)
	b.books = books
	b.errs = errs
	b.repriceAll(now)
	return b, nil
}

func (a *Adapter) matchMarkets(ctx context.Context, matchSlug string) ([]pricedMarket, error) {
	sport, known := a.sports[sportCodeOfSlug(matchSlug)]
	if !known {
		a.deps.Log().Warn("match belongs to an unmapped sport", "source", Name, "event_id", matchSlug)
		return nil, nil
	}
	query := url.Values{"slug": make([]string, 0, len(matchEventSuffixes))}
	for _, suffix := range matchEventSuffixes {
		query["slug"] = append(query["slug"], matchSlug+suffix)
	}
	var events []gammaEvent
	if err := a.deps.GetJSON(ctx, a.deps.Config.BaseURL+"/events?"+query.Encode(), &events); err != nil {
		return nil, fmt.Errorf("match events %s: %w", matchSlug, err)
	}
	teams, ok := teamsOf(events)
	if !ok {
		a.deps.Log().Warn("match has no home and away teams", "source", Name, "event_id", matchSlug)
		return nil, nil
	}
	builder := newMatchBuilder(matchSlug, teams, marketsBySport[sport], func(sourceMarketID string) {
		a.deps.DropUnknownMarket(Name, sourceMarketID)
	})
	for _, ev := range events {
		builder.addEvent(ev)
	}
	return builder.markets, nil
}

func (a *Adapter) fetchBooks(ctx context.Context, tokens []string) (map[string]*ladder, map[string]bool, error) {
	books := make(map[string]*ladder, len(tokens))
	unbooked := map[string]bool{}
	var errs []error
	for batchTokens := range slices.Chunk(tokens, booksPerRequest) {
		batch, err := a.fetchBookBatch(ctx, batchTokens)
		if err != nil {
			errs = append(errs, err)
			for _, token := range batchTokens {
				unbooked[token] = true
			}
			continue
		}
		for _, book := range batch {
			books[book.AssetID] = newLadder(book.Bids, book.Asks)
		}
	}
	return books, unbooked, errors.Join(errs...)
}

func (a *Adapter) fetchBookBatch(ctx context.Context, tokens []string) ([]clobBook, error) {
	payload := make([]bookRequest, len(tokens))
	for i, t := range tokens {
		payload[i] = bookRequest{TokenID: t}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode book request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.clobURL()+"/books", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build book request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.deps.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch books: %w", err)
	}
	var books []clobBook
	if err := json.Unmarshal(resp, &books); err != nil {
		return nil, fmt.Errorf("decode clob books: %w", err)
	}
	return books, nil
}

func (a *Adapter) clobURL() string {
	if u := a.deps.Config.Extra["clobUrl"]; u != "" {
		return u
	}
	return defaultCLOBURL
}
