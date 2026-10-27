package stats

import (
	"context"
	"time"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
)

func (s *Service) collectMatchStats(ctx context.Context, now time.Time) {
	if !s.isEnabled(sofascore.SourceName) && !s.isEnabled(fotmob.SourceName) {
		return
	}
	window := StatsWindow{
		From: now.Add(-statsRetryWindow), To: now.Add(-statsDelayAfterKickoff), Limit: statsEventsPerTick,
		Skip: recentlyAttempted(s.lastStatsAttempt, now, statsRetryInterval),
	}
	events, err := s.store.EventsAwaitingStats(ctx, window, s.isEnabled(sofascore.SourceName))
	if err != nil {
		s.recordFailure("postgres", "match_stats", "err", err)
		return
	}
	for _, e := range events {
		if !isDue(s.lastStatsAttempt, e.ID, now, statsRetryInterval) {
			continue
		}
		s.lastStatsAttempt[e.ID] = now
		itemCtx, cancel := context.WithTimeout(ctx, jobItemBudget)
		s.collectEventStats(itemCtx, e)
		cancel()
	}
}

func (s *Service) collectEventStats(ctx context.Context, e ScheduledEvent) {
	if s.isEnabled(sofascore.SourceName) {
		isStored, err := s.statsFromSofascore(ctx, e)
		if err != nil {
			s.recordFailure(sofascore.SourceName, "match_stats", "event_id", e.ID, "err", err)
		}
		if isStored {
			return
		}
	}
	if e.HasAnyStatsRows || !s.isEnabled(fotmob.SourceName) {
		return
	}
	if err := s.statsFromFotMob(ctx, e); err != nil {
		s.recordFailure(fotmob.SourceName, "match_stats", "event_id", e.ID, "err", err)
	}
}

func (s *Service) statsFromSofascore(ctx context.Context, e ScheduledEvent) (bool, error) {
	if e.SofascoreID == nil {
		return false, nil
	}
	event, err := s.sofascore.Event(ctx, *e.SofascoreID)
	if err != nil {
		return false, err
	}
	if err := s.store.SetEventStatus(ctx, e.ID, string(seed.StatusFromSofascore(event.Status.Type))); err != nil {
		return false, err
	}
	if !event.IsFinished() {
		return false, nil
	}
	lineups, err := s.sofascore.Lineups(ctx, *e.SofascoreID)
	if err != nil {
		return false, err
	}
	incidents, err := s.sofascore.Incidents(ctx, *e.SofascoreID)
	if err != nil {
		return false, err
	}
	lines := sofascore.MatchLines(lineups, incidents)
	sided := make([]sidedPlayer, len(lines))
	for i, l := range lines {
		sided[i] = sidedPlayer{player: l.Player, isHome: l.IsHome}
	}
	playerIDs, err := s.ensureSofascorePlayers(ctx, e, sided)
	if err != nil {
		return false, err
	}
	earlier, err := s.fallbackReadingsByPlayer(ctx, e)
	if err != nil {
		return false, err
	}
	stats := make([]MatchStat, 0, len(lines))
	for _, l := range lines {
		playerID := playerIDs[l.Player.ID]
		merged := Merge(sofascoreReading(l), earlier[playerID])
		s.logDisagreements(e.ID, playerID, merged.Disagreements)
		stats = append(stats, statFor(e, playerID, l.IsHome, merged))
	}
	if err := s.store.UpsertMatchStats(ctx, stats); err != nil {
		return false, err
	}
	s.log.Info("match stats stored", "source", sofascore.SourceName, "event_id", e.ID, "players", len(stats))
	return true, nil
}

func (s *Service) fallbackReadingsByPlayer(ctx context.Context, e ScheduledEvent) (map[string]Reading, error) {
	if !e.HasAnyStatsRows {
		return nil, nil
	}
	existing, err := s.store.EventMatchStats(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	readings := make(map[string]Reading, len(existing))
	for _, m := range existing {
		readings[m.PlayerID] = readingFromStored(m, fotmob.SourceName)
	}
	return readings, nil
}

func (s *Service) statsFromFotMob(ctx context.Context, e ScheduledEvent) error {
	details, isFound, err := s.fotmobDetails(ctx, e)
	if err != nil || !isFound || !details.IsFinished() {
		return err
	}
	_, err = s.storeFotMobStats(ctx, e, details)
	return err
}

func (s *Service) storeFotMobStats(ctx context.Context, e ScheduledEvent, details fotmob.Details) (int, error) {
	starters := map[int64]bool{}
	for _, entry := range details.LineupEntries() {
		starters[entry.FotmobID] = !entry.Substitute && !entry.Missing
	}
	lines := details.PlayerLines()
	refs := make([]fotmobPlayerRef, len(lines))
	for i, l := range lines {
		refs[i] = fotmobPlayerRef{id: l.FotmobID, name: l.Name, isHome: l.IsHome}
	}
	playerIDs, err := s.fotmobPlayerIDs(ctx, e, refs)
	if err != nil {
		return 0, err
	}
	stats := make([]MatchStat, 0, len(lines))
	for _, l := range lines {
		playerID, ok := playerIDs[l.FotmobID]
		if !ok {
			continue
		}
		merged := Merge(fotmobReading(l, starters[l.FotmobID]), Reading{})
		stats = append(stats, statFor(e, playerID, l.IsHome, merged))
	}
	if err := s.store.UpsertMatchStats(ctx, stats); err != nil {
		return 0, err
	}
	if err := s.store.SetEventStatus(ctx, e.ID, string(seed.EventFinished)); err != nil {
		return 0, err
	}
	s.log.Info("match stats stored", "source", fotmob.SourceName, "event_id", e.ID, "players", len(stats), "unmatched", len(lines)-len(stats))
	return len(stats), nil
}

func (s *Service) logDisagreements(eventID, playerID string, disagreements []Disagreement) {
	for _, d := range disagreements {
		s.log.Info("stat sources disagree, sofascore wins", "source", sofascore.SourceName, "event_id", eventID,
			"player_id", playerID, "field", d.Field, "sofascore", d.Primary, "other", d.Secondary)
	}
}

func statFor(e ScheduledEvent, playerID string, isHome bool, merged Merged) MatchStat {
	stat := MatchStat{PlayerID: playerID, EventID: e.ID, IsHome: isHome, PlayedAt: e.Kickoff, TeamID: e.AwayTeamID, OpponentTeamID: e.HomeTeamID}
	if isHome {
		stat.TeamID, stat.OpponentTeamID = e.HomeTeamID, e.AwayTeamID
	}
	merged.applyTo(&stat)
	return stat
}

func sofascoreReading(l sofascore.PlayerLine) Reading {
	values := map[Field]float64{
		FieldMinutes: float64(l.Minutes), FieldStarted: boolValue(l.Started),
		FieldYellow: float64(l.Yellow), FieldRed: float64(l.Red),
	}
	if l.HasStatistics {
		values[FieldGoals], values[FieldAssists] = float64(l.Goals), float64(l.Assists)
		values[FieldShots], values[FieldShotsOnTarget] = float64(l.Shots), float64(l.ShotsOnTarget)
		values[FieldFouls], values[FieldTackles] = float64(l.Fouls), float64(l.Tackles)
		if l.Rating != nil {
			values[FieldRating] = *l.Rating
		}
	}
	return Reading{Source: sofascore.SourceName, Values: values}
}

func fotmobReading(l fotmob.PlayerLine, isStarter bool) Reading {
	return Reading{Source: fotmob.SourceName, Values: map[Field]float64{
		FieldMinutes: float64(l.Minutes), FieldStarted: boolValue(isStarter),
		FieldGoals: float64(l.Goals), FieldAssists: float64(l.Assists),
		FieldShots: float64(l.Shots), FieldShotsOnTarget: float64(l.ShotsOnTarget),
		FieldFouls: float64(l.Fouls), FieldTackles: float64(l.Tackles),
		FieldYellow: float64(l.Yellow), FieldRed: float64(l.Red),
	}}
}

func readingFromStored(m MatchStat, source string) Reading {
	all := map[Field]float64{
		FieldMinutes: float64(m.Minutes), FieldStarted: boolValue(m.Started), FieldGoals: float64(m.Goals),
		FieldAssists: float64(m.Assists), FieldShots: float64(m.Shots), FieldShotsOnTarget: float64(m.ShotsOnTarget),
		FieldFouls: float64(m.Fouls), FieldTackles: float64(m.Tackles), FieldYellow: float64(m.Yellow), FieldRed: float64(m.Red),
	}
	values := map[Field]float64{}
	for field, value := range all {
		if m.ValueSources[string(field)] == source {
			values[field] = value
		}
	}
	return Reading{Source: source, Values: values}
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
