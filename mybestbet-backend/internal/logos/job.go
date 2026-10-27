package logos

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	StaleAfter      = 7 * 24 * time.Hour
	FailureBackoff  = 24 * time.Hour
	jobInterval     = time.Hour
	fetchBatch      = 200
	fetchWorkers    = 4
	fetchRatePerSec = 8
)

type JobDeps struct {
	Store       *Store
	Fetcher     *Fetcher
	SourceLogos map[string]string
	Log         *slog.Logger
	Now         func() time.Time
}

type Job struct {
	deps    JobDeps
	limiter *rate.Limiter
}

type PassResult struct {
	Stored    int
	Unchanged int
	Failed    int
}

func NewJob(deps JobDeps) *Job {
	return &Job{deps: deps, limiter: rate.NewLimiter(fetchRatePerSec, 1)}
}

func (j *Job) Run(ctx context.Context) error {
	ticker := time.NewTicker(jobInterval)
	defer ticker.Stop()
	for {
		result, err := j.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			j.deps.Log.ErrorContext(ctx, "logo fetch pass failed", "err", err)
		} else {
			j.deps.Log.InfoContext(ctx, "logo fetch pass", "stored", result.Stored, "unchanged", result.Unchanged, "failed", result.Failed)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (j *Job) RunOnce(ctx context.Context) (PassResult, error) {
	if err := j.registerSources(ctx); err != nil {
		return PassResult{}, err
	}
	var result PassResult
	for {
		now := j.deps.Now()
		due, err := j.deps.Store.due(ctx, now.Add(-StaleAfter), now.Add(-FailureBackoff), fetchBatch)
		if err != nil {
			return result, err
		}
		if err := j.fetchAll(ctx, due, &result); err != nil {
			return result, err
		}
		if len(due) < fetchBatch {
			return result, nil
		}
	}
}

func (j *Job) registerSources(ctx context.Context) error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(j.deps.SourceLogos)) {
		errs = append(errs, j.deps.Store.UpsertCandidate(ctx, j.deps.Store.pool, KindSource, name, j.deps.SourceLogos[name]))
	}
	return errors.Join(errs...)
}

func (j *Job) fetchAll(ctx context.Context, due []candidate, result *PassResult) error {
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(fetchWorkers)
	for _, c := range due {
		group.Go(func() error {
			if err := j.limiter.Wait(groupCtx); err != nil {
				return err
			}
			outcome, err := j.fetchOne(groupCtx, c)
			mu.Lock()
			result.add(outcome)
			mu.Unlock()
			return err
		})
	}
	return group.Wait()
}

type fetchOutcome int

const (
	outcomeSkipped fetchOutcome = iota
	outcomeStored
	outcomeUnchanged
	outcomeFailed
)

func (r *PassResult) add(outcome fetchOutcome) {
	switch outcome {
	case outcomeStored:
		r.Stored++
	case outcomeUnchanged:
		r.Unchanged++
	case outcomeFailed:
		r.Failed++
	default:
	}
}

func (j *Job) fetchOne(ctx context.Context, c candidate) (fetchOutcome, error) {
	image, err := j.deps.Fetcher.Fetch(ctx, c.SourceURL)
	if err != nil {
		if ctx.Err() != nil {
			return outcomeSkipped, ctx.Err()
		}
		j.deps.Log.WarnContext(ctx, "logo fetch failed", "kind", c.Kind, "entity_id", c.EntityID, "url", c.SourceURL, "err", err)
		return outcomeFailed, j.deps.Store.saveFailure(ctx, c, err, j.deps.Now())
	}
	changed, err := j.deps.Store.saveFetched(ctx, c, image, j.deps.Now())
	if err != nil || !changed {
		return outcomeUnchanged, err
	}
	return outcomeStored, nil
}
