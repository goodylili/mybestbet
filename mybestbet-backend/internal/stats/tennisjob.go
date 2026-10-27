package stats

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/internal/stats/statkey"
)

const (
	tennisRoundInterval    = 30 * time.Minute
	tennisBackfillInterval = 24 * time.Hour
)

var tennisRecentDayOffsets = []int{-1, 0}

type TennisCollector struct {
	flashscore     *flashscore.Client
	store          *Store
	entities       *seed.Store
	leagues        []string
	log            *slog.Logger
	now            func() time.Time
	nextBackfillAt time.Time
}

type TennisCollectorDeps struct {
	Flashscore *flashscore.Client
	Store      *Store
	Entities   *seed.Store
	Leagues    []string
	Log        *slog.Logger
	Now        func() time.Time
}

type competitorIDs struct {
	teamID   string
	playerID string
}

func NewTennisCollector(d TennisCollectorDeps) *TennisCollector {
	c := &TennisCollector{flashscore: d.Flashscore, store: d.Store, entities: d.Entities, log: d.Log, now: d.Now}
	for _, code := range d.Leagues {
		if _, hasTournament := flashscore.Tournaments[code]; hasTournament {
			c.leagues = append(c.leagues, code)
		}
	}
	return c
}

func (c *TennisCollector) Run(ctx context.Context) error {
	ticker := time.NewTicker(tennisRoundInterval)
	defer ticker.Stop()
	for {
		c.CollectRound(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (c *TennisCollector) CollectRound(ctx context.Context) {
	now := c.now()
	isBackfill := !now.Before(c.nextBackfillAt)
	if isBackfill {
		c.nextBackfillAt = now.Add(tennisBackfillInterval)
	}
	candidates, err := c.finishedMatches(ctx, isBackfill)
	if err != nil {
		recordJobFailure(c.log, flashscore.SourceName, "tennis_matches", "err", err)
	}
	pending, err := c.matchesWithoutStats(ctx, candidates)
	if err != nil {
		recordJobFailure(c.log, "postgres", "tennis_stats", "err", err)
		return
	}
	pending = c.inKnownLeagues(ctx, pending)
	stored := 0
	for _, m := range pending {
		if ctx.Err() != nil {
			return
		}
		itemCtx, cancel := context.WithTimeout(ctx, jobItemBudget)
		err := c.storeMatchStats(itemCtx, m)
		cancel()
		switch {
		case errors.Is(err, flashscore.ErrNoStats):
		case err != nil:
			recordJobFailure(c.log, flashscore.SourceName, "tennis_stats", "match_id", m.ID, "err", err)
		default:
			stored++
		}
	}
	c.log.Info("tennis stats round", "source", flashscore.SourceName, "backfill", isBackfill,
		"candidates", len(candidates), "pending", len(pending), "stored", stored)
}

func (c *TennisCollector) finishedMatches(ctx context.Context, isBackfill bool) ([]flashscore.Match, error) {
	offsets := tennisRecentDayOffsets
	if isBackfill {
		offsets = nil
		for offset := flashscore.OldestDayOffset; offset <= 0; offset++ {
			offsets = append(offsets, offset)
		}
	}
	var lists [][]flashscore.Match
	var errs []error
	for _, offset := range offsets {
		matches, err := c.flashscore.DayMatches(ctx, offset)
		lists, errs = append(lists, matches), append(errs, err)
	}
	if isBackfill {
		for _, code := range c.leagues {
			matches, err := c.flashscore.TournamentResults(ctx, code)
			lists, errs = append(lists, matches), append(errs, err)
		}
	}
	return uniqueFinished(lists), errors.Join(errs...)
}

func uniqueFinished(lists [][]flashscore.Match) []flashscore.Match {
	seen := map[string]bool{}
	var finished []flashscore.Match
	for _, matches := range lists {
		for _, m := range matches {
			if m.Status == flashscore.StatusFinished && !seen[m.ID] {
				seen[m.ID] = true
				finished = append(finished, m)
			}
		}
	}
	return finished
}

func (c *TennisCollector) matchesWithoutStats(ctx context.Context, matches []flashscore.Match) ([]flashscore.Match, error) {
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m.ID)
	}
	withStats, err := c.store.SourceEventsWithStats(ctx, flashscore.SourceName, ids)
	if err != nil {
		return nil, err
	}
	pending := matches[:0:0]
	for _, m := range matches {
		if !withStats[m.ID] {
			pending = append(pending, m)
		}
	}
	return pending, nil
}

func (c *TennisCollector) inKnownLeagues(ctx context.Context, matches []flashscore.Match) []flashscore.Match {
	known := map[string]bool{}
	kept := matches[:0:0]
	for _, m := range matches {
		code := m.Tournament.Code
		if _, checked := known[code]; !checked {
			err := c.store.EnsureTennisLeague(ctx, m.Tournament)
			if err != nil {
				recordJobFailure(c.log, "postgres", "tennis_league", "league", code, "err", err)
			}
			known[code] = err == nil
		}
		if known[code] {
			kept = append(kept, m)
		}
	}
	return kept
}

func (c *TennisCollector) storeMatchStats(ctx context.Context, m flashscore.Match) error {
	serve, err := c.flashscore.ServeStats(ctx, m.ID)
	if err != nil {
		return err
	}
	home, err := c.competitor(ctx, m.LeagueCode, m.Home)
	if err != nil {
		return err
	}
	away, err := c.competitor(ctx, m.LeagueCode, m.Away)
	if err != nil {
		return err
	}
	eventID, err := c.entities.UpsertSourceEvent(ctx, seed.SourceEvent{
		Source: flashscore.SourceName, SourceID: m.ID, LeagueCode: m.LeagueCode,
		HomeTeamID: home.teamID, AwayTeamID: away.teamID, Kickoff: m.Kickoff, Status: seed.EventFinished,
	})
	if err != nil {
		return err
	}
	return c.store.UpsertMatchStats(ctx, []MatchStat{
		tennisMatchStat(eventID, m.Kickoff, home, away, true, serve.Home),
		tennisMatchStat(eventID, m.Kickoff, away, home, false, serve.Away),
	})
}

func (c *TennisCollector) competitor(ctx context.Context, leagueCode string, p flashscore.Player) (competitorIDs, error) {
	teamID, playerID, err := c.entities.UpsertCompetitor(ctx, seed.FlashscoreCompetitor(leagueCode, p))
	return competitorIDs{teamID: teamID, playerID: playerID}, err
}

func tennisMatchStat(eventID string, playedAt time.Time, player, opponent competitorIDs, isHome bool, serve flashscore.ServeStats) MatchStat {
	return MatchStat{
		PlayerID: player.playerID, EventID: eventID, TeamID: player.teamID, OpponentTeamID: opponent.teamID,
		IsHome: isHome, PlayedAt: playedAt, Started: true,
		Stats:        statkey.Values{statkey.Aces: float64(serve.Aces), statkey.DoubleFaults: float64(serve.DoubleFaults)},
		ValueSources: map[string]string{"stats": flashscore.SourceName},
	}
}
