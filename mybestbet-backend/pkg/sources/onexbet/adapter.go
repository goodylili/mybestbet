package onexbet

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

type brand struct {
	name           string
	defaultPartner string
	webURL         string
}

var brands = []brand{
	{name: "onexbet", defaultPartner: "159", webURL: "https://1xbet.ng"},
	{name: "22bet", defaultPartner: "151", webURL: "https://22bet.ng"},
	{name: "paripesa", defaultPartner: "188", webURL: "https://paripesa.ng"},
	{name: "betwinner", defaultPartner: "152", webURL: "https://betwinner.ng"},
}

const (
	partnerExtraKey      = "partner"
	countryExtraKey      = "country"
	bookingBaseExtraKey  = "bookingBaseUrl"
	webURLExtraKey       = "webUrl"
	nigeriaCountryID     = "132"
	maxEventsPerGame     = "2000"
	requestTimeout       = 15 * time.Second
	minPollInterval      = 5 * time.Second
	concurrentEventFetch = 4
)

func init() {
	for _, b := range brands {
		sources.Register(b.name, b.factory())
	}
}

type Adapter struct {
	name         string
	partner      string
	country      string
	bookingBase  string
	webURL       string
	retryDelay   time.Duration
	deps         sources.Deps
	competitions map[string]competition
}

func (b brand) factory() sources.Factory {
	return func(deps sources.Deps) sources.Adapter {
		return &Adapter{
			name:         b.name,
			partner:      extraOrDefault(deps.Config, partnerExtraKey, b.defaultPartner),
			country:      extraOrDefault(deps.Config, countryExtraKey, nigeriaCountryID),
			bookingBase:  extraOrDefault(deps.Config, bookingBaseExtraKey, deps.Config.BaseURL),
			webURL:       extraOrDefault(deps.Config, webURLExtraKey, b.webURL),
			retryDelay:   saveCouponRetryDelay,
			deps:         deps,
			competitions: competitionsWithOverrides(deps.Config.LeagueIDs),
		}
	}
}

func extraOrDefault(cfg sources.Config, key, fallback string) string {
	if v := cfg.Extra[key]; v != "" {
		return v
	}
	return fallback
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     suppliedCodes(),
		Leagues:     slices.Sorted(maps.Keys(a.competitions)),
		Geo:         a.deps.Config.Geo,
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(a.name, a.deps.Clock())
	listings := map[int][]championshipSummary{}
	for _, league := range leagues {
		c, ok := a.competitions[league]
		if !ok {
			continue
		}
		championshipIDs, err := a.championshipIDsFor(ctx, c, listings)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", league, err))
			continue
		}
		for _, championshipID := range championshipIDs {
			games, err := a.championshipGames(ctx, championshipID)
			if err != nil {
				snap.Errors = append(snap.Errors, fmt.Errorf("discover %s championship %s: %w", league, championshipID, err))
				continue
			}
			for _, g := range games {
				if g.isPlainFixture() {
					snap.Events = append(snap.Events, sourceEvent(g, league, championshipID))
				}
			}
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) championshipIDsFor(ctx context.Context, c competition, listings map[int][]championshipSummary) ([]string, error) {
	if c.byName == nil {
		return []string{c.championshipID}, nil
	}
	listing, ok := listings[c.sportID]
	if !ok {
		var resp response[[]championshipSummary]
		if err := a.getJSON(ctx, a.championshipsURL(c.sportID), &resp); err != nil {
			return nil, err
		}
		if err := resp.err(); err != nil {
			return nil, err
		}
		listing = resp.Value
		listings[c.sportID] = listing
	}
	var ids []string
	for _, summary := range listing {
		if c.byName.matches(summary.Name) {
			ids = append(ids, strconv.FormatInt(summary.ID, 10))
		}
	}
	return ids, nil
}

func (a *Adapter) championshipGames(ctx context.Context, championshipID string) ([]game, error) {
	var resp response[championship]
	if err := a.getJSON(ctx, a.championshipURL(championshipID), &resp); err != nil {
		return nil, err
	}
	return resp.Value.Games, resp.err()
}

func sourceEvent(g game, league, championshipID string) canon.SourceEvent {
	if g.LeagueID != 0 {
		championshipID = strconv.FormatInt(g.LeagueID, 10)
	}
	return canon.SourceEvent{
		SourceID:       strconv.FormatInt(g.ID, 10),
		LeagueHint:     league,
		SourceLeagueID: championshipID,
		Home:           g.Home,
		Away:           g.Away,
		Kickoff:        time.Unix(g.StartUnix, 0).UTC(),
	}
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	fetchedAt := a.deps.Clock()
	snap := sources.NewSnapshot(a.name, fetchedAt)
	perEvent := make([][]canon.RawMarket, len(eventIDs))
	perEventErr := make([]error, len(eventIDs))
	var group errgroup.Group
	group.SetLimit(concurrentEventFetch)
	for i, eventID := range eventIDs {
		group.Go(func() error {
			perEvent[i], perEventErr[i] = a.eventMarkets(ctx, eventID, fetchedAt)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return snap, err
	}
	for i := range eventIDs {
		snap.Markets = append(snap.Markets, perEvent[i]...)
		if perEventErr[i] != nil {
			snap.Errors = append(snap.Errors, perEventErr[i])
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) eventMarkets(ctx context.Context, eventID string, fetchedAt time.Time) ([]canon.RawMarket, error) {
	acc := newMarketAccumulator()
	_, subGameErr, err := a.walkEvent(ctx, a.deps.Config.BaseURL, eventID, fetchedAt, func(o offer) {
		acc.add(eventID, o.where.period, o.game, o.group, o.rule, o.placed)
	})
	if err != nil {
		return nil, fmt.Errorf("price event %s: %w", eventID, err)
	}
	if subGameErr != nil {
		return acc.rawMarkets(), fmt.Errorf("price event %s: %w", eventID, subGameErr)
	}
	return acc.rawMarkets(), nil
}

type offer struct {
	game   game
	where  sportScope
	group  int
	rule   groupRule
	bet    bet
	placed placedBet
}

func (a *Adapter) walkEvent(
	ctx context.Context,
	baseURL string,
	eventID string,
	observedAt time.Time,
	visit func(offer),
) (main game, subGameErr, err error) {
	main, err = a.game(ctx, baseURL, eventID)
	if err != nil {
		return game{}, nil, err
	}
	sport := sportOfLineFeedID[main.SportID]
	a.collect(main, sportScope{sport: sport, scope: fullMatch}, observedAt, visit)
	var subGameErrs []error
	for _, sub := range main.SubGames {
		target, ok := subGameTargets[sport][sub.slot()]
		if !ok {
			continue
		}
		g, err := a.game(ctx, baseURL, strconv.FormatInt(sub.ID, 10))
		if err != nil {
			subGameErrs = append(subGameErrs, fmt.Errorf("sub game %d: %w", sub.ID, err))
			continue
		}
		g.inheritCompetitors(main)
		a.collect(g, sportScope{sport: sport, scope: target.scope, period: target.period}, observedAt, visit)
	}
	return main, errors.Join(subGameErrs...), nil
}

func (a *Adapter) collect(g game, where sportScope, observedAt time.Time, visit func(offer)) {
	for _, ge := range g.Groups {
		rule, ok := groupRules[groupKey{sport: where.sport, scope: where.scope, group: ge.Group}]
		if !ok {
			a.deps.DropUnknownMarket(a.name, fmt.Sprintf("%d:%d", g.ID, ge.Group))
			continue
		}
		for _, column := range ge.Columns {
			for _, b := range column {
				if bool(b.Blocked) || b.Odds <= 1 {
					continue
				}
				if placed, ok := rule.place(b, g, observedAt); ok {
					visit(offer{game: g, where: where, group: ge.Group, rule: rule, bet: b, placed: placed})
				}
			}
		}
	}
}

func (a *Adapter) game(ctx context.Context, baseURL, gameID string) (game, error) {
	var resp response[game]
	if err := a.getJSON(ctx, a.gameURL(baseURL, gameID), &resp); err != nil {
		return game{}, err
	}
	return resp.Value, resp.err()
}

func (a *Adapter) championshipsURL(sportID int) string {
	return a.deps.Config.BaseURL + "/service-api/LineFeed/GetChampsZip?sport=" + strconv.Itoa(sportID) +
		"&lng=en&country=" + url.QueryEscape(a.country) + "&partner=" + url.QueryEscape(a.partner)
}

func (a *Adapter) championshipURL(championshipID string) string {
	return a.deps.Config.BaseURL + "/service-api/LineFeed/GetChampZip?champ=" + url.QueryEscape(championshipID) +
		"&lng=en&country=" + url.QueryEscape(a.country) + "&partner=" + url.QueryEscape(a.partner)
}

func (a *Adapter) gameURL(baseURL, gameID string) string {
	return baseURL + "/service-api/LineFeed/GetGameZip?id=" + url.QueryEscape(gameID) +
		"&lng=en&isSubGames=true&GroupEvents=true&countevents=" + maxEventsPerGame +
		"&country=" + url.QueryEscape(a.country) + "&partner=" + url.QueryEscape(a.partner)
}

func (a *Adapter) getJSON(ctx context.Context, rawURL string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return a.deps.GetJSON(ctx, rawURL, out)
}
