package kalshi

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name            = "kalshi"
	eventsPageLimit = "200"
	maxEventPages   = 10
	minPollInterval = 2 * time.Second
)

type Adapter struct {
	deps         sources.Deps
	leagues      map[string]league
	byGameSeries map[string]league
	series       seriesCache
}

func init() { sources.Register(Name, New) }

func New(deps sources.Deps) sources.Adapter {
	leagues := leaguesWithOverrides(deps.Config.LeagueIDs)
	return &Adapter{deps: deps, leagues: leagues, byGameSeries: leaguesByGameSeries(leagues)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     marketsOffered(a.leagues),
		Leagues:     slices.Sorted(maps.Keys(a.leagues)),
		Push:        a.deps.Config.APIKey != "",
		Geo:         a.deps.Config.Geo,
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	listings := map[string]eventList{}
	for _, code := range leagues {
		l, mapped := a.leagues[code]
		if !mapped {
			continue
		}
		gameSeries := l.gameSeries()
		listing, fetched := listings[gameSeries]
		if !fetched {
			var err error
			listing, err = a.listOpenEvents(ctx, gameSeries, url.Values{"with_milestones": {"true"}, "with_nested_markets": {"true"}})
			if err != nil {
				snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", code, err))
				continue
			}
			listings[gameSeries] = listing
		}
		snap.Events = append(snap.Events, a.leagueGames(listing, code, l)...)
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

func (a *Adapter) listOpenEvents(ctx context.Context, series string, extra url.Values) (eventList, error) {
	query := url.Values{"series_ticker": {series}, "status": {"open"}, "limit": {eventsPageLimit}}
	maps.Copy(query, extra)
	var all eventList
	for range maxEventPages {
		var page eventList
		if err := a.deps.GetJSON(ctx, a.deps.Config.BaseURL+"/events?"+query.Encode(), &page); err != nil {
			return eventList{}, fmt.Errorf("list events for %s: %w", series, err)
		}
		all.Events = append(all.Events, page.Events...)
		all.Milestones = append(all.Milestones, page.Milestones...)
		if page.Cursor == "" {
			break
		}
		query.Set("cursor", page.Cursor)
	}
	return all, nil
}

func (a *Adapter) leagueGames(listing eventList, code string, l league) []canon.SourceEvent {
	kickoffs := make(map[string]time.Time, len(listing.Milestones))
	for _, m := range listing.Milestones {
		for _, ticker := range append(m.PrimaryEventTickers, m.Details.MainGameEventTicker) {
			kickoffs[ticker] = m.StartDate.UTC()
		}
	}
	var games []canon.SourceEvent
	for _, ev := range listing.Events {
		if l.competition != "" && ev.ProductMetadata.Competition != l.competition {
			continue
		}
		players, hasPlayers := discoveredLineup(ev, l.family.awayListedFirst)
		kickoff, hasKickoff := kickoffs[ev.EventTicker]
		if !hasKickoff {
			kickoff, hasKickoff = kickoffFromTicker(ev.EventTicker)
		}
		if !hasPlayers || !hasKickoff {
			a.deps.Log().Debug("game skipped without competitors or kickoff", "source", Name, "event_id", ev.EventTicker)
			continue
		}
		games = append(games, canon.SourceEvent{
			SourceID:   ev.EventTicker,
			LeagueHint: code,
			Home:       players.home.name,
			Away:       players.away.name,
			Kickoff:    kickoff,
		})
	}
	return games
}

func discoveredLineup(ev event, awayListedFirst bool) (lineup, bool) {
	if players, ok := lineupOf(ev, awayListedFirst); ok {
		return players, true
	}
	return lineupFromTitle(ev.Title, awayListedFirst)
}

func (a *Adapter) loadBoard(ctx context.Context, eventIDs []string) (*board, error) {
	now := a.deps.Clock()
	quotes := map[string]quote{}
	var markets []pricedMarket
	var errs []error
	for _, id := range eventIDs {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		gameMarkets, err := a.gameMarkets(ctx, id, quotes)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		markets = append(markets, gameMarkets...)
	}
	if len(markets) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	b := newBoard(markets, quotes, now)
	b.errs = errs
	return b, nil
}

var errNotGameTicker = errors.New("kalshi: event id is not a game event ticker")

func (a *Adapter) gameMarkets(ctx context.Context, gameTicker string, quotes map[string]quote) ([]pricedMarket, error) {
	gameSeries, key, _ := strings.Cut(gameTicker, "-")
	l, isGame := a.byGameSeries[gameSeries]
	if !isGame || key == "" {
		return nil, fmt.Errorf("%w: %q", errNotGameTicker, gameTicker)
	}
	drop := func(sourceMarketID string) { a.deps.DropUnknownMarket(Name, sourceMarketID) }
	game, found, err := a.openEvent(ctx, gameSeries, gameTicker)
	if err != nil || !found {
		return nil, err
	}
	players, ok := lineupOf(game, l.family.awayListedFirst)
	if !ok {
		drop(gameTicker)
		return nil, nil
	}
	var markets []pricedMarket
	for _, spec := range l.family.series {
		series := l.prefix + spec.suffix
		ev, found, err := a.openEvent(ctx, series, series+"-"+key+spec.eventSuffix)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		for _, m := range ev.Markets {
			quotes[m.Ticker] = m.quote()
		}
		markets = append(markets, buildEventMarkets(gameTicker, ev, spec, players, drop)...)
	}
	return markets, nil
}

func (a *Adapter) openEvent(ctx context.Context, series, eventTicker string) (event, bool, error) {
	now := a.deps.Clock()
	events, cached := a.series.get(series, now)
	if !cached {
		listing, err := a.listOpenEvents(ctx, series, url.Values{"with_nested_markets": {"true"}})
		if err != nil {
			return event{}, false, err
		}
		events = make(map[string]event, len(listing.Events))
		for _, ev := range listing.Events {
			events[ev.EventTicker] = ev
		}
		a.series.put(series, events, now)
	}
	ev, found := events[eventTicker]
	return ev, found, nil
}
