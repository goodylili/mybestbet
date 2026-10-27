package prizepicks

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	sourceName           = "prizepicks"
	projectionsPageSize  = "1000"
	projectionPreGame    = "pre_game"
	requestTimeout       = 20 * time.Second
	minInterval          = time.Minute
	extraSessionURL      = "sessionUrl"
	extraSessionTTL      = "sessionTtl"
	jsonAPIAcceptsHeader = "application/json"
)

type Adapter struct {
	deps                 sources.Deps
	sessions             *sessionCache
	leaguesByCompetition map[string]string
}

func init() { sources.Register(sourceName, New) }

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{
		deps: deps,
		sessions: &sessionCache{
			browser:    deps.Browser,
			sessionURL: extraOr(deps.Config, extraSessionURL, defaultSessionURL),
			ttl:        sessionTTL(deps.Config),
			now:        deps.Clock,
		},
		leaguesByCompetition: competitionLookup(deps.Config.LeagueIDs),
	}
}

func (a *Adapter) Name() string { return sourceName }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:      suppliedMarkets,
		Leagues:      a.supportedLeagues(),
		NeedsBrowser: true,
		Geo:          a.deps.Config.Geo,
		MinInterval:  minInterval,
	}
}

func (a *Adapter) supportedLeagues() []string {
	leagues := slices.Collect(maps.Values(a.leaguesByCompetition))
	for _, b := range leagueBoards {
		if !b.hasCompetitions {
			leagues = append(leagues, b.canonicalLeague)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(leagues)))
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(sourceName, a.deps.Clock())
	err := a.eachBoard(ctx, boardsForLeagues(leagues), &snap, func(lb leagueBoard, b board) {
		snap.Events = append(snap.Events, a.boardEvents(lb, b, leagues)...)
	})
	return snap, err
}

func (a *Adapter) boardEvents(lb leagueBoard, b board, wanted []string) []canon.SourceEvent {
	var events []canon.SourceEvent
	for _, gameID := range b.gameIDsWithProjections() {
		game := b.games[gameID]
		league := lb.leagueOf(game, a.leaguesByCompetition)
		if league == "" || !slices.Contains(wanted, league) {
			continue
		}
		events = append(events, canon.SourceEvent{
			SourceID:   lb.eventSourceID(gameID),
			LeagueHint: league,
			Home:       teamName(game.Metadata.GameInfo.Teams.Home),
			Away:       teamName(game.Metadata.GameInfo.Teams.Away),
			Kickoff:    game.StartTime.UTC(),
		})
	}
	return events
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(sourceName, a.deps.Clock())
	gameIDsByBoard := map[string][]string{}
	for _, eventID := range eventIDs {
		lb, gameID := splitEventSourceID(eventID)
		gameIDsByBoard[lb.name] = append(gameIDsByBoard[lb.name], gameID)
	}
	var boards []leagueBoard
	for _, lb := range leagueBoards {
		if len(gameIDsByBoard[lb.name]) > 0 {
			boards = append(boards, lb)
		}
	}
	err := a.eachBoard(ctx, boards, &snap, func(lb leagueBoard, b board) {
		snap.Markets = append(snap.Markets, a.boardMarkets(lb, b, gameIDsByBoard[lb.name], snap.FetchedAt)...)
	})
	return snap, err
}

func (a *Adapter) eachBoard(ctx context.Context, boards []leagueBoard, snap *canon.Snapshot, collect func(leagueBoard, board)) error {
	for _, lb := range boards {
		b, err := a.fetchBoard(ctx, lb)
		if err != nil {
			snap.Errors = append(snap.Errors, err)
			continue
		}
		collect(lb, b)
	}
	if len(boards) > 0 && len(snap.Errors) == len(boards) {
		return errors.Join(snap.Errors...)
	}
	return nil
}

func (a *Adapter) boardMarkets(lb leagueBoard, b board, gameIDs []string, observedAt time.Time) []canon.RawMarket {
	var markets []canon.RawMarket
	for _, p := range b.projections {
		gameID := p.Relationships.Game.id()
		if !slices.Contains(gameIDs, gameID) || p.Attributes.Status != projectionPreGame {
			continue
		}
		if market, ok := a.lineMarket(lb, b, p, observedAt); ok {
			markets = append(markets, market)
		}
	}
	return markets
}

func (a *Adapter) lineMarket(lb leagueBoard, b board, p projectionResource, observedAt time.Time) (canon.RawMarket, bool) {
	stat, ok := statMarketsBySport[lb.sport][normalize(p.Attributes.StatType)]
	if !ok {
		a.deps.DropUnknownMarket(sourceName, p.Attributes.StatType)
		return canon.RawMarket{}, false
	}
	oddsType := normalize(p.Attributes.OddsType)
	if oddsType == "" {
		oddsType = oddsTypeStandard
	}
	playerID := p.Relationships.NewPlayer.id()
	player, hasPlayer := b.players[playerID]
	if !knownOddsTypes[oddsType] || !hasPlayer {
		a.deps.DropUnknownMarket(sourceName, p.ID)
		return canon.RawMarket{}, false
	}
	return canon.RawMarket{
		EventSourceID: lb.eventSourceID(p.Relationships.Game.id()),
		Code:          stat.code,
		Period:        stat.period,
		Line:          canon.Line(p.Attributes.LineScore),
		Player:        &canon.SourcePlayer{SourceID: playerID, Name: firstNonEmpty(player.DisplayName, player.Name), Team: player.TeamName},
		Selections: []canon.Selection{{
			Side:  canon.SideOver,
			Label: oddsType,
			Price: canon.NewPrice(p.Attributes.LineScore, canon.FormLineOnly, observedAt),
		}},
		Rules:          stat.code.DefaultRule(),
		SourceMarketID: p.ID,
	}, true
}

func (a *Adapter) fetchBoard(ctx context.Context, lb leagueBoard) (board, error) {
	sess, err := a.sessions.get(ctx)
	if err != nil {
		return board{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.projectionsURL(lb.leagueID(a.deps.Config)), nil)
	if err != nil {
		return board{}, fmt.Errorf("build projections request: %w", err)
	}
	sess.apply(req)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", jsonAPIAcceptsHeader)
	}
	body, err := a.deps.Do(req)
	if errors.Is(err, sources.ErrForbidden) {
		a.sessions.invalidate()
	}
	if err != nil {
		return board{}, fmt.Errorf("fetch %s projections: %w", lb.name, err)
	}
	return decodeBoard(body)
}

func (a *Adapter) projectionsURL(leagueID string) string {
	query := url.Values{"league_id": {leagueID}, "per_page": {projectionsPageSize}, "single_stat": {"true"}}
	return strings.TrimRight(a.deps.Config.BaseURL, "/") + "/projections?" + query.Encode()
}

func (b board) gameIDsWithProjections() []string {
	seen := map[string]bool{}
	var ids []string
	for _, p := range b.projections {
		id := p.Relationships.Game.id()
		if _, known := b.games[id]; known && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func competitionLookup(overrides map[string]string) map[string]string {
	lookup := maps.Clone(defaultLeaguesByCompetition)
	for league, competition := range overrides {
		lookup[normalize(competition)] = league
	}
	return lookup
}

func sessionTTL(cfg sources.Config) time.Duration {
	if ttl, err := time.ParseDuration(cfg.Extra[extraSessionTTL]); err == nil && ttl > 0 {
		return ttl
	}
	return defaultSessionTTL
}

func extraOr(cfg sources.Config, key, fallback string) string {
	if v := cfg.Extra[key]; v != "" {
		return v
	}
	return fallback
}

func teamName(t gameTeam) string { return firstNonEmpty(t.Name, t.Abbreviation) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
