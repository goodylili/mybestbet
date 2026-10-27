package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"

	"mybestbet/internal/logos"
	"mybestbet/internal/signal"
	"mybestbet/pkg/canon"
)

const (
	maxEventsListed  = 1000
	maxHistoryPoints = 20000
)

const eventInfoColumns = `e.id::text, e."leagueCode", h.id::text, h.name, a.id::text, a.name, e.kickoff, e.status, e.score`

const eventInfoJoins = `FROM events e JOIN teams h ON h.id = e."homeTeamId" JOIN teams a ON a.id = e."awayTeamId"`

func scanEventInfo(row pgx.Row) (EventInfo, error) {
	var e EventInfo
	err := row.Scan(e.columnTargets()...)
	return e, err
}

func (e *EventInfo) columnTargets() []any {
	return []any{&e.ID, &e.LeagueCode, &e.Home.ID, &e.Home.Name, &e.Away.ID, &e.Away.Name, &e.Kickoff, &e.Status, &e.Score}
}

func (s *Server) fixtureCountsByLeague(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT "leagueCode", count(*) FROM events WHERE kickoff >= $1 AND status IN ('scheduled', 'live') AND "mergedInto" IS NULL GROUP BY 1`, since)
	if err != nil {
		return nil, fmt.Errorf("count fixtures: %w", err)
	}
	counts := map[string]int{}
	var league string
	var count int
	_, err = pgx.ForEachRow(rows, []any{&league, &count}, func() error {
		counts[league] = count
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan fixture counts: %w", err)
	}
	return counts, nil
}

type eventFilter struct {
	league      string
	leagueCodes []string
	from        time.Time
	to          time.Time
}

const eventFilterWhere = `e.kickoff >= $1 AND e.kickoff < $2 AND ($3 = '' OR e."leagueCode" = $3)
	AND ($4::text[] IS NULL OR e."leagueCode" = ANY($4)) AND e."mergedInto" IS NULL`

func (s *Server) eventDayCounts(ctx context.Context, filter eventFilter, loc *time.Location) ([]CalendarDay, error) {
	rows, err := s.pool.Query(ctx, `SELECT to_char(e.kickoff AT TIME ZONE $5, 'YYYY-MM-DD'), count(*) FROM events e WHERE `+eventFilterWhere+`
		GROUP BY 1 ORDER BY 1`, filter.from, filter.to, filter.league, filter.leagueCodes, loc.String())
	if err != nil {
		return nil, fmt.Errorf("count events per day: %w", err)
	}
	days, err := pgx.CollectRows(rows, pgx.RowToStructByPos[CalendarDay])
	if err != nil {
		return nil, fmt.Errorf("scan event day counts: %w", err)
	}
	return days, nil
}

func (s *Server) eventLeagues(ctx context.Context, eventIDs []string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, "leagueCode" FROM events WHERE id = ANY($1::uuid[])`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("list event leagues: %w", err)
	}
	leagues := map[string]string{}
	var eventID, league string
	_, err = pgx.ForEachRow(rows, []any{&eventID, &league}, func() error {
		leagues[eventID] = league
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan event leagues: %w", err)
	}
	return leagues, nil
}

func (s *Server) eventsBetween(ctx context.Context, filter eventFilter) ([]EventInfo, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+eventInfoColumns+` `+eventInfoJoins+` WHERE `+eventFilterWhere+`
		ORDER BY e.kickoff, e.id LIMIT $5`, filter.from, filter.to, filter.league, filter.leagueCodes, maxEventsListed)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (EventInfo, error) { return scanEventInfo(row) })
	if err != nil {
		return nil, fmt.Errorf("scan events: %w", err)
	}
	if err := s.attachEventLogos(ctx, events); err != nil {
		return nil, err
	}
	if err := s.attachEventLinks(ctx, s.withSports(events)); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Server) eventByID(ctx context.Context, id string) (EventInfo, error) {
	event, err := scanEventInfo(s.pool.QueryRow(ctx, `SELECT `+eventInfoColumns+` `+eventInfoJoins+` WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return EventInfo{}, ErrNotFound
	}
	if err != nil {
		return EventInfo{}, fmt.Errorf("load event: %w", err)
	}
	event.Sport = s.sportOf(event.LeagueCode)
	if err := s.attachTeamLogos(ctx, []*Team{&event.Home, &event.Away}); err != nil {
		return EventInfo{}, err
	}
	events := []EventInfo{event}
	if err := s.attachEventLinks(ctx, events); err != nil {
		return EventInfo{}, err
	}
	return events[0], nil
}

func (s *Server) badgesByEvent(ctx context.Context, eventIDs []string) (map[string]Badges, error) {
	rows, err := s.pool.Query(ctx, `SELECT "eventId"::text,
			count(*) FILTER (WHERE type = 'arb'), count(*) FILTER (WHERE type = 'ev'),
			coalesce(max(margin) FILTER (WHERE type = 'arb'), 0), coalesce(max(ev) FILTER (WHERE type = 'ev'), 0)
		FROM signals WHERE status = 'open' AND "eventId" = ANY($1::uuid[]) GROUP BY 1`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("list event badges: %w", err)
	}
	badges := map[string]Badges{}
	var eventID string
	var b Badges
	_, err = pgx.ForEachRow(rows, []any{&eventID, &b.Arbs, &b.EVs, &b.MaxMargin, &b.MaxEV}, func() error {
		badges[eventID] = b
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan event badges: %w", err)
	}
	return badges, nil
}

type signalFilter struct {
	signalType  string
	leagueCodes []string
	minimum     float64
	limit       int
}

func (s *Server) openSignals(ctx context.Context, filter signalFilter) ([]SignalView, error) {
	rows, err := s.pool.Query(ctx, `SELECT s.id::text, s."dedupeKey", s.type, s.status, s."leagueCode", s."marketKey",
			s.margin, s.ev, s."fairProb", s.legs, s."oldestLegAgeMs", s."detectedAt", s."closedAt", `+eventInfoColumns+`
		FROM signals s JOIN events e ON e.id = s."eventId" JOIN teams h ON h.id = e."homeTeamId" JOIN teams a ON a.id = e."awayTeamId"
		WHERE s.status = 'open' AND e."mergedInto" IS NULL AND ($1 = '' OR s.type = $1) AND greatest(s.margin, s.ev) >= $2
			AND ($3::text[] IS NULL OR s."leagueCode" = ANY($3))
		ORDER BY s."detectedAt" DESC LIMIT $4`, filter.signalType, filter.minimum, filter.leagueCodes, filter.limit)
	if err != nil {
		return nil, fmt.Errorf("list signals: %w", err)
	}
	signals, err := pgx.CollectRows(rows, scanSignalView)
	if err != nil {
		return nil, fmt.Errorf("scan signals: %w", err)
	}
	teams := make([]*Team, 0, 2*len(signals))
	for i := range signals {
		signals[i].Event.Sport = s.sportOf(signals[i].Event.LeagueCode)
		teams = append(teams, &signals[i].Event.Home, &signals[i].Event.Away)
	}
	if err := s.attachSignalPlayers(ctx, signals); err != nil {
		return nil, err
	}
	s.attachSignalLinks(signals)
	return signals, s.attachTeamLogos(ctx, teams)
}

func (s *Server) attachSignalPlayers(ctx context.Context, signals []SignalView) error {
	ids := []string{}
	for _, v := range signals {
		if v.Market.PlayerID != "" && !slices.Contains(ids, v.Market.PlayerID) {
			ids = append(ids, v.Market.PlayerID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	players, err := s.playersByID(ctx, ids)
	if err != nil {
		return err
	}
	for i := range signals {
		if player, ok := players[signals[i].Market.PlayerID]; ok {
			signals[i].Player = &player
		}
	}
	return nil
}

func scanSignalView(row pgx.CollectableRow) (SignalView, error) {
	var v SignalView
	var marketKey string
	var legs []byte
	var oldestLegAgeMs int64
	e := &v.Event
	err := row.Scan(append([]any{&v.ID, &v.DedupeKey, &v.Type, &v.Status, &v.LeagueCode, &marketKey,
		&v.Margin, &v.EV, &v.FairProb, &legs, &oldestLegAgeMs, &v.DetectedAt, &v.ClosedAt}, e.columnTargets()...)...)
	if err != nil {
		return v, err
	}
	v.EventID = e.ID
	v.OldestLegAge = time.Duration(oldestLegAgeMs) * time.Millisecond
	if v.Market, err = canon.ParseMarketKey(marketKey); err != nil {
		return v, err
	}
	if err := json.Unmarshal(legs, &v.Legs); err != nil {
		return v, fmt.Errorf("decode legs: %w", err)
	}
	v.Legs = emptyIfNil(v.Legs)
	fillLegMarkets(&v.Signal)
	signal.DescribeMove(&v.Signal)
	return v, nil
}

func fillLegMarkets(s *canon.Signal) {
	if s.Type == canon.SignalLine {
		return
	}
	key := s.Market.String()
	for i := range s.Legs {
		if s.Legs[i].MarketKey == "" {
			s.Legs[i].MarketKey, s.Legs[i].Line = key, s.Market.Line
		}
	}
}

func (s *Server) marketHistory(ctx context.Context, key canon.MarketKey, source string) ([]HistoryPoint, error) {
	rows, err := s.pool.Query(ctx, `SELECT source, side, label, raw, form, prob, suspended, "observedAt"
		FROM prices_history WHERE "eventId" = $1 AND "marketKey" = $2 AND ($3 = '' OR source = $3)
		ORDER BY "observedAt" LIMIT $4`, key.EventID, key.String(), source, maxHistoryPoints)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	points, err := pgx.CollectRows(rows, pgx.RowToStructByPos[HistoryPoint])
	if err != nil {
		return nil, fmt.Errorf("scan history: %w", err)
	}
	return points, nil
}

func (s *Server) playersByID(ctx context.Context, ids []string) (map[string]PropPlayer, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id::text, p.name, coalesce(p."teamId"::text, ''), coalesce(p.position, ''), coalesce(t.name, '')
		FROM players p LEFT JOIN teams t ON t.id = p."teamId" WHERE p.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	players, err := pgx.CollectRows(rows, pgx.RowToStructByPos[PropPlayer])
	if err != nil {
		return nil, fmt.Errorf("scan players: %w", err)
	}
	teamIDs := make([]string, 0, len(players))
	for _, p := range players {
		if p.TeamID != "" {
			teamIDs = append(teamIDs, p.TeamID)
		}
	}
	logoURLs, err := s.logoURLs(ctx, logos.KindTeam, teamIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]PropPlayer, len(players))
	for _, p := range players {
		p.TeamLogoURL = logoURLs[p.TeamID]
		byID[p.ID] = p
	}
	return byID, nil
}

func (s *Server) playerByID(ctx context.Context, id string) (PropPlayer, error) {
	players, err := s.playersByID(ctx, []string{id})
	if err != nil {
		return PropPlayer{}, err
	}
	player, ok := players[id]
	if !ok {
		return PropPlayer{}, ErrNotFound
	}
	return player, nil
}
