package stats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/internal/seed"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/canon"
)

const (
	DefaultPlayerMergeInterval = 6 * time.Hour
	playerMergeAnnouncer       = "player_merge"
)

var playerIdentitySources = []string{
	fotmob.SourceName, sofascore.SourceName, espn.SourceName, flashscore.SourceName, euroleague.SourceName,
	acb.SourceName, bo3.SourceName, esports.OpenDotaSource,
}

type PlayerMerger struct {
	store    *Store
	fotmob   *fotmob.Client
	redis    *goredis.Client
	enabled  func(source string) bool
	interval time.Duration
	log      *slog.Logger
}

type PlayerMergerDeps struct {
	Store    *Store
	FotMob   *fotmob.Client
	Redis    *goredis.Client
	Enabled  func(source string) bool
	Interval time.Duration
	Log      *slog.Logger
}

type PlayerMergeReport struct {
	Locked       bool
	NamePairs    int
	Evidenced    int
	Merged       int
	MovedAliases int
	Ambiguous    []string
	Failed       int
}

type playerMerge struct {
	duplicateID string
	survivorID  string
	name        string
}

type playerMergePlan struct {
	merges    []playerMerge
	ambiguous []string
	evidenced int
}

func NewPlayerMerger(d PlayerMergerDeps) *PlayerMerger {
	interval := d.Interval
	if interval <= 0 {
		interval = DefaultPlayerMergeInterval
	}
	return &PlayerMerger{store: d.Store, fotmob: d.FotMob, redis: d.Redis, enabled: d.Enabled, interval: interval, log: d.Log}
}

func (m *PlayerMerger) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		m.mergeRound(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (m *PlayerMerger) mergeRound(ctx context.Context) {
	report, err := m.MergeDuplicates(ctx)
	if err != nil && ctx.Err() == nil {
		recordJobFailure(m.log, "postgres", "player_merge", "err", err, "merged", report.Merged, "failed", report.Failed)
		return
	}
	m.log.Info("duplicate players merged", "locked", report.Locked, "name_pairs", report.NamePairs, "evidenced", report.Evidenced,
		"merged", report.Merged, "moved_aliases", report.MovedAliases, "ambiguous", len(report.Ambiguous), "skipped_names", report.Ambiguous)
}

func (m *PlayerMerger) MergeDuplicates(ctx context.Context) (PlayerMergeReport, error) {
	var report PlayerMergeReport
	locked, err := m.store.withPlayerMergeLock(ctx, func(conn *pgxpool.Conn) error {
		twins, err := playerTwins(ctx, conn, seed.NationalTeamLeagues(), playerIdentitySources, fotmob.SourceName)
		if err != nil {
			return err
		}
		plan := planPlayerMerges(twins, m.squadClubs(ctx, twins))
		report.NamePairs, report.Evidenced, report.Ambiguous = len(twins), plan.evidenced, plan.ambiguous
		var errs []error
		for _, pm := range plan.merges {
			result, err := mergePlayerInto(ctx, conn, pm.duplicateID, pm.survivorID)
			if err != nil {
				report.Failed++
				errs = append(errs, err)
				continue
			}
			if !result.merged {
				continue
			}
			report.Merged++
			report.MovedAliases += len(result.movedAliases)
			errs = append(errs, m.announce(ctx, pm, result.movedAliases))
		}
		return errors.Join(errs...)
	})
	report.Locked = locked
	if err != nil {
		return report, fmt.Errorf("merge duplicate players: %w", err)
	}
	return report, nil
}

func (m *PlayerMerger) squadClubs(ctx context.Context, twins []playerTwin) map[string]map[string]string {
	clubs := map[string]map[string]string{}
	if m.fotmob == nil || (m.enabled != nil && !m.enabled(fotmob.SourceName)) {
		return clubs
	}
	for _, t := range twins {
		team := t.nationalFotmobTeam
		if t.playedForClub || t.hasConflictingIdent || t.duplicateFotmobID == "" || team == "" {
			continue
		}
		if _, fetched := clubs[team]; fetched {
			continue
		}
		clubs[team] = map[string]string{}
		squad, err := m.fotmob.TeamSquad(ctx, team)
		if err != nil {
			recordJobFailure(m.log, fotmob.SourceName, "player_merge_squad", "team", team, "err", err)
			continue
		}
		for _, p := range squad {
			if p.ClubID != "" {
				clubs[team][p.ID] = p.ClubID
			}
		}
	}
	return clubs
}

func planPlayerMerges(twins []playerTwin, squadClubs map[string]map[string]string) playerMergePlan {
	var evidenced []playerTwin
	for _, t := range twins {
		if !t.hasConflictingIdent && (t.playedForClub || isListedAtClub(t, squadClubs)) {
			evidenced = append(evidenced, t)
		}
	}
	perDuplicate, perSurvivor := map[string]int{}, map[string]int{}
	for _, t := range evidenced {
		perDuplicate[t.duplicateID]++
		perSurvivor[t.survivorID]++
	}
	plan := playerMergePlan{evidenced: len(evidenced)}
	for _, t := range evidenced {
		if perDuplicate[t.duplicateID] == 1 && perSurvivor[t.survivorID] == 1 {
			plan.merges = append(plan.merges, playerMerge{duplicateID: t.duplicateID, survivorID: t.survivorID, name: t.name})
			continue
		}
		if !slices.Contains(plan.ambiguous, t.name) {
			plan.ambiguous = append(plan.ambiguous, t.name)
		}
	}
	return plan
}

func isListedAtClub(t playerTwin, squadClubs map[string]map[string]string) bool {
	club := squadClubs[t.nationalFotmobTeam][t.duplicateFotmobID]
	return club != "" && slices.Contains(t.clubFotmobTeams, club)
}

func (m *PlayerMerger) announce(ctx context.Context, pm playerMerge, aliases []movedAlias) error {
	if m.redis == nil {
		return nil
	}
	updates := make([]canon.AliasUpdate, 0, len(aliases)+1)
	updates = append(updates, canon.AliasUpdate{Kind: canon.AliasPlayerMerge, Source: playerMergeAnnouncer, SourceID: pm.duplicateID, TargetID: pm.survivorID})
	for _, a := range aliases {
		updates = append(updates, canon.AliasUpdate{Kind: canon.AliasPlayer, Source: a.source, SourceID: a.sourceID, TargetID: pm.survivorID})
	}
	for _, u := range updates {
		body, err := json.Marshal(u)
		if err != nil {
			return fmt.Errorf("encode player merge update: %w", err)
		}
		if err := m.redis.Publish(ctx, redis.ChannelAliasUpdate, body).Err(); err != nil {
			return fmt.Errorf("publish player merge update: %w", err)
		}
	}
	return nil
}
