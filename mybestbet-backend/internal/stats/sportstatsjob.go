package stats

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/statkey"
)

const (
	esportsStatsDelay    = 8 * time.Hour
	seriesGameLeadWindow = time.Hour
	dotaLeagueCode       = "DOTA2"
	backfillEventLimit   = 2000
)

var (
	errStatsNotFinal      = errors.New("stats: game not final yet")
	errMalformedSourceID  = errors.New("stats: malformed event source id")
	sportStatEventSources = []string{espn.SourceName, euroleague.SourceName, esports.LiquipediaSource, bo3.SourceName, acb.SourceName}
	sportStatLeagueCodes  = slices.Concat(slices.Collect(maps.Keys(espn.Leagues)), slices.Collect(maps.Keys(euroleague.Competitions)),
		slices.Collect(maps.Keys(bo3.Disciplines)), []string{dotaLeagueCode, acb.LeagueCode})
	esportsEventSources = []string{esports.LiquipediaSource, bo3.SourceName}
)

type sourceLine struct {
	sourcePlayerID string
	name           string
	isHome         bool
	minutes        int
	started        bool
	values         statkey.Values
}

type sportLines struct {
	playerSource string
	lines        []sourceLine
}

func (s *Service) collectSportStats(ctx context.Context, now time.Time) {
	s.collectSportStatsSince(ctx, now, now.Add(-statsRetryWindow), statsEventsPerTick)
}

func (s *Service) BackfillSportStats(ctx context.Context, lookback time.Duration) {
	now := s.now()
	s.collectSportStatsSince(ctx, now, now.Add(-lookback), backfillEventLimit)
}

func (s *Service) collectSportStatsSince(ctx context.Context, now, oldestKickoff time.Time, limit int) {
	window := StatsWindow{
		From: oldestKickoff, To: now.Add(-statsDelayAfterKickoff), Limit: limit,
		Skip: recentlyAttempted(s.lastStatsAttempt, now, statsRetryInterval),
	}
	s.collectSportStatsIn(ctx, now, window, sportStatLeagueCodes, s.lastStatsAttempt)
}

func (s *Service) collectSportStatsIn(ctx context.Context, now time.Time, window StatsWindow, leagueCodes []string, attempts map[string]time.Time) {
	events, err := s.store.EventsAwaitingSportStats(ctx, window, leagueCodes, s.enabledEventSources())
	if err != nil {
		s.recordFailure("postgres", "sport_stats", "err", err)
		return
	}
	for _, e := range events {
		if slices.Contains(esportsEventSources, e.Source) && now.Sub(e.Kickoff) < esportsStatsDelay {
			continue
		}
		if !isDue(attempts, e.ID, now, statsRetryInterval) {
			continue
		}
		attempts[e.ID] = now
		if !s.isEnabled(statSourceOf(e)) {
			continue
		}
		itemCtx, cancel := context.WithTimeout(ctx, jobItemBudget)
		if err := s.collectSportEventStats(itemCtx, e); err != nil && !errors.Is(err, errStatsNotFinal) {
			s.recordFailure(e.Source, "sport_stats", "event_id", e.ID, "err", err)
		}
		cancel()
	}
}

func (s *Service) enabledEventSources() []string {
	enabled := make([]string, 0, len(sportStatEventSources))
	for _, source := range sportStatEventSources {
		isLiquipediaReadable := source == esports.LiquipediaSource && (s.isEnabled(esports.OpenDotaSource) || s.isEnabled(bo3.SourceName))
		if isLiquipediaReadable || s.isEnabled(source) {
			enabled = append(enabled, source)
		}
	}
	return enabled
}

func statSourceOf(e SourcedEvent) string {
	if e.Source != esports.LiquipediaSource {
		return e.Source
	}
	if e.LeagueCode == dotaLeagueCode {
		return esports.OpenDotaSource
	}
	return bo3.SourceName
}

func (s *Service) collectSportEventStats(ctx context.Context, e SourcedEvent) error {
	lines, err := s.sportLinesFor(ctx, e)
	if err != nil {
		return err
	}
	stats, err := s.matchStatsFor(ctx, e, lines)
	if err != nil {
		return err
	}
	if err := s.store.UpsertMatchStats(ctx, stats); err != nil {
		return err
	}
	if err := s.store.SetEventStatus(ctx, e.ID, string(seed.EventFinished)); err != nil {
		return err
	}
	s.log.Info("match stats stored", "source", lines.playerSource, "event_id", e.ID, "players", len(stats))
	return nil
}

func (s *Service) sportLinesFor(ctx context.Context, e SourcedEvent) (sportLines, error) {
	switch e.Source {
	case espn.SourceName:
		return s.espnLines(ctx, e)
	case euroleague.SourceName:
		return s.euroLeagueLines(ctx, e)
	case esports.LiquipediaSource:
		if e.LeagueCode == dotaLeagueCode {
			return s.dotaLines(ctx, e)
		}
		return s.bo3Lines(ctx, e)
	case bo3.SourceName:
		return s.bo3Lines(ctx, e)
	case acb.SourceName:
		return s.acbLines(ctx, e)
	}
	return sportLines{}, fmt.Errorf("%s: %w", e.Source, errMalformedSourceID)
}

func (s *Service) matchStatsFor(ctx context.Context, e SourcedEvent, sl sportLines) ([]MatchStat, error) {
	sourceIDs := make([]string, 0, len(sl.lines))
	for _, l := range sl.lines {
		sourceIDs = append(sourceIDs, l.sourcePlayerID)
	}
	playerIDs, err := s.store.PlayerIDsBySourceID(ctx, sl.playerSource, sourceIDs)
	if err != nil {
		return nil, err
	}
	stats := make([]MatchStat, 0, len(sl.lines))
	for _, l := range sl.lines {
		teamID, opponentID := e.AwayTeamID, e.HomeTeamID
		if l.isHome {
			teamID, opponentID = e.HomeTeamID, e.AwayTeamID
		}
		playerID, known := playerIDs[l.sourcePlayerID]
		if !known {
			playerID, err = s.entities.UpsertSourcePlayer(ctx, seed.SourcePlayer{Source: sl.playerSource, SourceID: l.sourcePlayerID, TeamID: teamID, Name: l.name})
			if err != nil {
				return nil, err
			}
		}
		stats = append(stats, MatchStat{
			PlayerID: playerID, EventID: e.ID, TeamID: teamID, OpponentTeamID: opponentID, IsHome: l.isHome, PlayedAt: e.Kickoff,
			Minutes: l.minutes, Started: l.started, Stats: l.values,
			ValueSources: map[string]string{string(FieldMinutes): sl.playerSource, "stats": sl.playerSource},
		})
	}
	return stats, nil
}

func (s *Service) espnLines(ctx context.Context, e SourcedEvent) (sportLines, error) {
	leaguePrefix, gameID, found := strings.Cut(e.SourceID, ":")
	if !found || !strings.EqualFold(leaguePrefix, e.LeagueCode) {
		return sportLines{}, fmt.Errorf("%s: %w", e.SourceID, errMalformedSourceID)
	}
	box, err := s.espn.Boxscore(ctx, e.LeagueCode, gameID)
	if err != nil {
		return sportLines{}, err
	}
	if !box.IsFinal() {
		return sportLines{}, s.markInProgress(ctx, e, box.Status)
	}
	sl := sportLines{playerSource: espn.SourceName}
	for _, l := range box.Lines {
		sl.lines = append(sl.lines, sourceLine{
			sourcePlayerID: seed.ESPNSourceID(e.LeagueCode, l.Athlete.ID), name: l.Athlete.Name,
			isHome: l.TeamID == box.HomeTeamID, minutes: l.Minutes, started: l.Started, values: l.Values,
		})
	}
	return sl, nil
}

func (s *Service) markInProgress(ctx context.Context, e SourcedEvent, espnStatus string) error {
	if err := s.store.SetEventStatus(ctx, e.ID, string(seed.StatusFromESPN(espnStatus))); err != nil {
		return err
	}
	return errStatsNotFinal
}

func (s *Service) euroLeagueLines(ctx context.Context, e SourcedEvent) (sportLines, error) {
	seasonCode, gameCode, err := euroLeagueGameOf(e)
	if err != nil {
		return sportLines{}, err
	}
	lines, err := s.euroleague.GameStats(ctx, seasonCode, gameCode)
	if err != nil {
		return sportLines{}, err
	}
	if len(lines) == 0 {
		return sportLines{}, errStatsNotFinal
	}
	sl := sportLines{playerSource: euroleague.SourceName}
	for _, l := range lines {
		sl.lines = append(sl.lines, sourceLine{
			sourcePlayerID: l.Player.Code, name: l.Player.Name, isHome: l.IsHome, minutes: l.Minutes, started: l.Started, values: l.Values,
		})
	}
	return sl, nil
}
