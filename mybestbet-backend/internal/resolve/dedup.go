package resolve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"mybestbet/pkg/canon"
)

const (
	aliasEventMerge    = "eventMerge"
	maxMergeChainDepth = 16
)

type DedupConfig struct {
	Every      time.Duration `yaml:"every"`
	PurgeGrace time.Duration `yaml:"purgeGrace"`
	PurgeBatch int           `yaml:"purgeBatch"`
}

var DefaultDedupConfig = DedupConfig{Every: 5 * time.Minute, PurgeGrace: 10 * time.Minute, PurgeBatch: 50}

func (c DedupConfig) withDefaults() DedupConfig {
	if c.Every <= 0 {
		c.Every = DefaultDedupConfig.Every
	}
	if c.PurgeGrace < 0 {
		c.PurgeGrace = DefaultDedupConfig.PurgeGrace
	}
	if c.PurgeBatch <= 0 {
		c.PurgeBatch = DefaultDedupConfig.PurgeBatch
	}
	return c
}

func LoadDedupConfig(configPath string) (DedupConfig, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return DedupConfig{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	var file struct {
		Resolve struct {
			Dedup *DedupConfig `yaml:"dedup"`
		} `yaml:"resolve"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return DedupConfig{}, fmt.Errorf("parse resolve.dedup block of %s: %w", configPath, err)
	}
	if file.Resolve.Dedup == nil {
		return DefaultDedupConfig, nil
	}
	return file.Resolve.Dedup.withDefaults(), nil
}

type PurgeReport struct {
	Batches            int
	Selected           int
	PurgedEvents       int
	StampedLegacy      int
	MovedLinks         int
	MovedSources       int
	MovedLineups       int
	MovedStats         int
	MovedClosingPrices int
	DroppedPrices      int
	DroppedHistory     int
	DroppedSignals     int
}

func (p *PurgeReport) add(o PurgeReport) {
	p.Batches++
	p.Selected += o.Selected
	p.PurgedEvents += o.PurgedEvents
	p.MovedLinks += o.MovedLinks
	p.MovedSources += o.MovedSources
	p.MovedLineups += o.MovedLineups
	p.MovedStats += o.MovedStats
	p.MovedClosingPrices += o.MovedClosingPrices
	p.DroppedPrices += o.DroppedPrices
	p.DroppedHistory += o.DroppedHistory
	p.DroppedSignals += o.DroppedSignals
}

type EventLinkMove struct {
	Source        string
	SourceEventID string
	Link          EventLink
}

type DedupReport struct {
	Ran   bool
	Merge MergeReport
	Purge PurgeReport
}

func (r *Resolver) DedupEvents(ctx context.Context, cfg DedupConfig) (DedupReport, error) {
	cfg = cfg.withDefaults()
	var report DedupReport
	ran, err := r.store.withJobLock(ctx, func() error {
		merge, mergeErr := r.MergeDuplicateEvents(ctx)
		report.Merge = merge
		purge, purgeErr := r.PurgeMergedEvents(ctx, cfg.PurgeGrace, cfg.PurgeBatch)
		report.Purge = purge
		return errors.Join(mergeErr, purgeErr)
	})
	report.Ran = ran
	return report, err
}

func (r *Resolver) PurgeMergedEvents(ctx context.Context, grace time.Duration, batchSize int) (PurgeReport, error) {
	return r.purgeMergedEvents(ctx, grace, batchSize, nil)
}

func (r *Resolver) purgeMergedEvents(ctx context.Context, grace time.Duration, batchSize int, scope []string) (PurgeReport, error) {
	var report PurgeReport
	stamped, err := r.store.stampLegacyMerges(ctx)
	if err != nil {
		return report, err
	}
	report.StampedLegacy = stamped
	mergedBefore := r.now().Add(-grace)
	for {
		var (
			batch PurgeReport
			moves []EventLinkMove
		)
		err := r.store.inMergeLock(ctx, func(tx mergeTx) error {
			var err error
			batch, moves, err = tx.purgeBatch(ctx, mergedBefore, batchSize, scope)
			return err
		})
		if err != nil {
			return report, fmt.Errorf("purge merged events: %w", err)
		}
		if batch.Selected == 0 {
			return report, nil
		}
		report.add(batch)
		countPurge(batch)
		if err := r.announceLinkMoves(ctx, moves); err != nil {
			return report, err
		}
		if batch.Selected < batchSize || batch.PurgedEvents == 0 {
			return report, nil
		}
	}
}

func (r *Resolver) announceLinkMoves(ctx context.Context, moves []EventLinkMove) error {
	errs := make([]error, 0, len(moves))
	for _, mv := range moves {
		errs = append(errs, r.announce(ctx, canon.AliasUpdate{Kind: canon.AliasEvent, Source: mv.Source, SourceID: mv.SourceEventID, TargetID: mv.Link.EventID, Flipped: mv.Link.Flipped}))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("announce purged event links: %w", err)
	}
	return nil
}

type DedupJob struct {
	resolver *Resolver
	cfg      DedupConfig
}

func NewDedupJob(r *Resolver, cfg DedupConfig) *DedupJob {
	return &DedupJob{resolver: r, cfg: cfg.withDefaults()}
}

func (j *DedupJob) Run(ctx context.Context) error {
	ticker := time.NewTicker(j.cfg.Every)
	defer ticker.Stop()
	for {
		j.runOnce(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (j *DedupJob) runOnce(ctx context.Context) {
	start := time.Now()
	report, err := j.resolver.DedupEvents(ctx, j.cfg)
	observeStage(stageDedup, start)
	log := j.resolver.log
	switch {
	case err != nil && ctx.Err() == nil:
		countDedupRun(dedupFailed)
		log.Warn("event dedup run failed", "error", err, "merged_events", report.Merge.MergedEvents, "purged_events", report.Purge.PurgedEvents)
		return
	case err != nil:
		return
	case !report.Ran:
		countDedupRun(dedupSkipped)
		log.Info("event dedup run skipped, another replica holds the lock")
		return
	}
	countDedupRun(dedupDone)
	p := report.Purge
	log.Info("event dedup run finished",
		"took_ms", time.Since(start).Milliseconds(),
		"merge_groups", report.Merge.Groups, "merged_events", report.Merge.MergedEvents,
		"moved_links", report.Merge.MovedLinks+p.MovedLinks, "closed_signals", report.Merge.ClosedSignals,
		"purge_batches", p.Batches, "purged_events", p.PurgedEvents, "stamped_legacy", p.StampedLegacy,
		"moved_sources", p.MovedSources, "moved_lineups", p.MovedLineups, "moved_stats", p.MovedStats,
		"moved_closing_prices", p.MovedClosingPrices, "dropped_prices", p.DroppedPrices,
		"dropped_history", p.DroppedHistory, "dropped_signals", p.DroppedSignals)
}
