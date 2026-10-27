package bayse

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name             = "bayse"
	publicKeyHeader  = "X-Public-Key"
	priceCurrency    = "NGN"
	unplayedStatus   = "UNPLAYED"
	gamesPageSize    = 100
	eventsPageSize   = 20
	maxPages         = 20
	booksPerRequest  = 20
	bookDepth        = "10"
	minPollInterval  = 2 * time.Second
	outcomeIDsParam  = "outcomeId[]"
	sportGameIDParam = "sportGameSlug"
	subcategoryParam = "subcategory"
)

type Adapter struct {
	deps    sources.Deps
	leagues leagueDirectory
}

func init() { sources.Register(Name, New) }

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{deps: deps, leagues: leagueDirectoryWithOverrides(deps.Config.LeagueIDs)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     supportedMarkets,
		Leagues:     a.leagues.codes(),
		Push:        true,
		Geo:         a.deps.Config.Geo,
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	for _, league := range leagues {
		events, err := a.discoverLeague(ctx, league, snap.FetchedAt)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", league, err))
			continue
		}
		snap.Events = append(snap.Events, events...)
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) discoverLeague(ctx context.Context, league string, now time.Time) ([]canon.SourceEvent, error) {
	if name, isGameLeague := a.leagues.gameLeagues[league]; isGameLeague {
		return a.upcomingGameEvents(ctx, league, name)
	}
	if subcategory, isListed := a.leagues.subcategories[league]; isListed {
		return a.upcomingListedEvents(ctx, league, subcategory, now)
	}
	return nil, nil
}

func (a *Adapter) upcomingGameEvents(ctx context.Context, league, leagueName string) ([]canon.SourceEvent, error) {
	games, err := a.upcomingGames(ctx, leagueName)
	if err != nil {
		return nil, err
	}
	events := make([]canon.SourceEvent, 0, len(games))
	for _, g := range games {
		events = append(events, canon.SourceEvent{
			SourceID:   g.Slug,
			LeagueHint: league,
			Home:       g.HomeTeam.Name,
			Away:       g.AwayTeam.Name,
			Kickoff:    g.StartDate.UTC(),
		})
	}
	return events, nil
}

func (a *Adapter) upcomingListedEvents(ctx context.Context, league, subcategory string, now time.Time) ([]canon.SourceEvent, error) {
	listed, err := a.listEvents(ctx, url.Values{subcategoryParam: {subcategory}})
	if err != nil {
		return nil, err
	}
	var events []canon.SourceEvent
	for _, g := range listedGamesOf(listed) {
		if g.game.Status != openStatus || !g.kickoff.After(now) {
			continue
		}
		events = append(events, canon.SourceEvent{
			SourceID:   g.game.Slug,
			LeagueHint: league,
			Home:       g.home,
			Away:       g.away,
			Kickoff:    g.kickoff,
		})
	}
	return events, nil
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	b, err := a.loadBoard(ctx, eventIDs)
	if err != nil {
		return canon.Snapshot{}, err
	}
	return b.snapshot(), nil
}

func (a *Adapter) upcomingGames(ctx context.Context, leagueName string) ([]game, error) {
	query := url.Values{"league": {leagueName}, "size": {strconv.Itoa(gamesPageSize)}}
	var games []game
	for page := 1; page <= maxPages; page++ {
		query.Set("page", strconv.Itoa(page))
		var list gameList
		if err := a.getJSON(ctx, "/v1/pm/sports/games?"+query.Encode(), &list); err != nil {
			return nil, fmt.Errorf("list games: %w", err)
		}
		for _, g := range list.Games {
			if g.Status == unplayedStatus {
				games = append(games, g)
			}
		}
		if page >= list.Pagination.LastPage {
			break
		}
	}
	return games, nil
}

func (a *Adapter) loadBoard(ctx context.Context, eventIDs []string) (*board, error) {
	now := a.deps.Clock()
	b := newBoard(now)
	drop := func(sourceMarketID string) { a.deps.DropUnknownMarket(Name, sourceMarketID) }
	listed, err := a.listedGamesFor(ctx, eventIDs)
	if err != nil {
		b.errs = append(b.errs, err)
	}
	for _, matchID := range eventIDs {
		if err := ctx.Err(); err != nil {
			b.errs = append(b.errs, err)
			break
		}
		events, codes, err := a.matchEvents(ctx, matchID, listed)
		if err != nil {
			b.errs = append(b.errs, err)
			continue
		}
		builder := newMatchBuilder(matchID, codes, drop)
		for _, ev := range mostLiquidFirst(events) {
			before := len(builder.markets)
			if builder.addEvent(ev) {
				b.addEvent(ev, builder.markets[before:])
			}
		}
	}
	if len(b.markets) == 0 && len(b.errs) > 0 {
		return nil, errors.Join(b.errs...)
	}
	if err := a.loadBooks(ctx, b); err != nil {
		return nil, err
	}
	b.repriceAll(now)
	return b, nil
}

func mostLiquidFirst(events []event) []event {
	sorted := slices.Clone(events)
	slices.SortStableFunc(sorted, func(x, y event) int { return cmp.Compare(y.Liquidity, x.Liquidity) })
	return sorted
}

func (a *Adapter) matchEvents(ctx context.Context, matchID string, listed map[string]listedGame) ([]event, matchCodes, error) {
	if codes, isGameSlug := codesOfGameSlug(matchID); isGameSlug {
		events, err := a.listEvents(ctx, url.Values{sportGameIDParam: {matchID}})
		if err != nil {
			return nil, matchCodes{}, fmt.Errorf("events for game %s: %w", matchID, err)
		}
		return events, codes, nil
	}
	g, found := listed[matchID]
	if !found {
		return nil, matchCodes{}, nil
	}
	return g.events(), g.codes(), nil
}

func (a *Adapter) listedGamesFor(ctx context.Context, eventIDs []string) (map[string]listedGame, error) {
	if !slices.ContainsFunc(eventIDs, isListedGameID) {
		return nil, nil
	}
	var games []listedGame
	for _, subcategory := range slices.Sorted(maps.Values(a.leagues.subcategories)) {
		events, err := a.listEvents(ctx, url.Values{subcategoryParam: {subcategory}})
		if err != nil {
			return nil, fmt.Errorf("events for %s: %w", subcategory, err)
		}
		games = append(games, listedGamesOf(events)...)
	}
	return listedGamesBySlug(games), nil
}

func isListedGameID(eventID string) bool {
	_, isGameSlug := codesOfGameSlug(eventID)
	return !isGameSlug
}

func (a *Adapter) listEvents(ctx context.Context, query url.Values) ([]event, error) {
	query.Set("currency", priceCurrency)
	query.Set("size", strconv.Itoa(eventsPageSize))
	var events []event
	for page := 1; page <= maxPages; page++ {
		query.Set("page", strconv.Itoa(page))
		var list eventList
		if err := a.getJSON(ctx, "/v1/pm/events?"+query.Encode(), &list); err != nil {
			return nil, fmt.Errorf("list events: %w", err)
		}
		events = append(events, list.Events...)
		if page >= list.Pagination.LastPage {
			break
		}
	}
	return events, nil
}

func (a *Adapter) loadBooks(ctx context.Context, b *board) error {
	outcomeIDs := b.clobOutcomeIDs()
	for start := 0; start < len(outcomeIDs); start += booksPerRequest {
		query := url.Values{outcomeIDsParam: outcomeIDs[start:min(start+booksPerRequest, len(outcomeIDs))], "depth": {bookDepth}}
		var books []outcomeBook
		if err := a.getJSON(ctx, "/v1/pm/books?"+query.Encode(), &books); err != nil {
			return fmt.Errorf("order books: %w", err)
		}
		for _, book := range books {
			b.books[book.OutcomeID] = quoteOf(book)
		}
	}
	return nil
}

func (a *Adapter) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.deps.Config.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	if key := a.deps.Config.APIKey; key != "" {
		req.Header.Set(publicKeyHeader, key)
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
