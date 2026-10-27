package ingest

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"slices"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/resolve"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	opDiscover          = "discover"
	opPrice             = "price"
	opStream            = "stream"
	defaultDiscovery    = 10 * time.Minute
	minDiscoveryTimeout = 2 * time.Minute
	discoveryPerLeague  = 3 * time.Second
	pollTick            = 500 * time.Millisecond
	healthWriteInterval = 2 * time.Second
	healthWriteTimeout  = time.Second
	resyncChunk         = 25
	hotChunk            = 4
	coolChunk           = 8
	requestCostWeight   = 0.2
	minRequestCost      = 0.05
)

type runner struct {
	name           string
	adapter        sources.Adapter
	streamer       sources.Streamer
	leagues        []string
	tiers          []config.Tier
	discoveryEvery time.Duration
	ceilingRPS     float64
	resolver       *resolve.Resolver
	rdb            *goredis.Client
	log            *slog.Logger
	now            func() time.Time
	unitRandom     func() float64

	schedule      *schedule
	hashes        *marketHashes
	health        *health
	focus         *focusSet
	requestCost   atomic.Uint64
	isStreamLive  atomic.Bool
	streamRestart chan struct{}
}

func (r *runner) run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return r.discoverPeriodically(ctx) })
	g.Go(func() error { return r.pollPricesContinuously(ctx) })
	g.Go(func() error { return r.writeHealthPeriodically(ctx) })
	if r.streamer != nil {
		g.Go(func() error { return r.holdStream(ctx) })
	}
	return g.Wait()
}

func (r *runner) discoverPeriodically(ctx context.Context) error {
	ticker := time.NewTicker(r.discoveryEvery)
	defer ticker.Stop()
	for {
		r.discover(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *runner) discover(ctx context.Context) {
	if r.health.isPaused(r.now()) {
		return
	}
	fetchCtx, cancel := context.WithTimeout(ctx, discoveryTimeoutFor(len(r.leagues), r.discoveryEvery))
	start := time.Now()
	snap, err := r.adapter.Discover(fetchCtx, r.leagues)
	cancel()
	r.recordFetch(ctx, opDiscover, start, snap, err)
	if err != nil && len(snap.Events) == 0 {
		return
	}
	events := r.upcoming(snap.Events)
	linked, err := r.resolver.LinkEvents(ctx, r.name, events)
	if err != nil {
		r.log.Warn("event linking incomplete", "source", r.name, "error", err)
	}
	if err := r.resolver.RecordSourceLeagues(ctx, r.name, events); err != nil {
		r.log.Warn("source leagues not recorded", "source", r.name, "error", err)
	}
	r.recordLinking(len(snap.Events), events, linked)
	if r.schedule.replace(events) {
		r.hashes.retainEvents(r.schedule.ids())
		r.signalStreamRestart()
	}
}

func discoveryTimeoutFor(leagues int, discoveryEvery time.Duration) time.Duration {
	budget := max(minDiscoveryTimeout, time.Duration(leagues)*discoveryPerLeague)
	return min(budget, max(minDiscoveryTimeout, discoveryEvery))
}

func (r *runner) upcoming(events []canon.SourceEvent) []canon.SourceEvent {
	now := r.now()
	return slices.DeleteFunc(slices.Clone(events), func(ev canon.SourceEvent) bool {
		return ev.Kickoff.Before(now)
	})
}

func (r *runner) recordLinking(discovered int, upcoming []canon.SourceEvent, linkedIDs []string) {
	linked := make(map[string]bool, len(linkedIDs))
	for _, id := range linkedIDs {
		linked[id] = true
	}
	unlinkedByLeague := map[string]int{}
	unlinked := 0
	var latest time.Time
	for _, ev := range upcoming {
		if !linked[ev.SourceID] {
			unlinkedByLeague[ev.LeagueHint]++
			unlinked++
		}
		if ev.Kickoff.After(latest) {
			latest = ev.Kickoff
		}
	}
	discoveredEvents.WithLabelValues(r.name, linkState).Set(float64(len(upcoming) - unlinked))
	discoveredEvents.WithLabelValues(r.name, unlinkedState).Set(float64(unlinked))
	r.log.Info("discovery linked",
		"source", r.name,
		"discovered", discovered,
		"upcoming", len(upcoming),
		"linked", len(upcoming)-unlinked,
		"latest_kickoff", latest,
		"unlinked_by_league", unlinkedByLeague,
	)
}

func (r *runner) pollPricesContinuously(ctx context.Context) error {
	var lanes errgroup.Group
	lanes.Go(func() error { return r.pollEvery(ctx, r.pollDueHot) })
	lanes.Go(func() error { return r.pollEvery(ctx, r.pollDueCool) })
	return lanes.Wait()
}

func (r *runner) pollEvery(ctx context.Context, poll func(context.Context)) error {
	ticker := time.NewTicker(pollTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !r.health.isPaused(r.now()) {
				poll(ctx)
			}
		}
	}
}

func (r *runner) pollDueHot(ctx context.Context) {
	now := r.now()
	intervals := r.currentIntervals(now)
	due := r.schedule.takeHot(now, r.tiers, intervals, r.unitRandom, r.focusPredicate())
	for chunk := range slices.Chunk(due, hotChunk) {
		if ctx.Err() != nil || r.health.isPaused(r.now()) {
			return
		}
		r.fetchPrices(ctx, fetchTimeoutFor(intervals[hotTier]), chunk)
	}
}

func (r *runner) pollDueCool(ctx context.Context) {
	now := r.now()
	intervals := r.currentIntervals(now)
	due := r.schedule.takeCool(now, r.tiers, intervals, r.unitRandom, coolChunk, r.focusPredicate())
	if ids := slices.Concat(due...); len(ids) > 0 {
		r.fetchPrices(ctx, maxFetchTimeout, ids)
	}
}

func (r *runner) focusPredicate() func(string) bool {
	if r.focus == nil || r.focus.isEmpty() {
		return nil
	}
	return func(sourceEventID string) bool {
		eventID, ok := r.resolver.LinkedEventID(r.name, sourceEventID)
		return ok && r.focus.has(eventID)
	}
}

func (r *runner) currentIntervals(now time.Time) []time.Duration {
	counts := r.schedule.eventsPerTier(r.tiers, now)
	intervals := stretchToCeiling(r.tiers, counts, r.eventCeiling())
	if r.isStreamLive.Load() {
		for i := range intervals {
			intervals[i] = streamReconcileInterval
		}
	}
	for i, t := range r.tiers {
		tierEvents.WithLabelValues(r.name, t.Name).Set(float64(counts[i]))
		tierInterval.WithLabelValues(r.name, t.Name).Set(intervals[i].Seconds())
	}
	return intervals
}

func (r *runner) eventCeiling() float64 {
	cost := math.Float64frombits(r.requestCost.Load())
	if cost <= 0 {
		return r.ceilingRPS
	}
	return r.ceilingRPS / max(cost, minRequestCost)
}

func (r *runner) fetchPrices(ctx context.Context, timeout time.Duration, eventIDs []string) {
	var sent atomic.Int64
	fetchCtx, cancel := context.WithTimeout(withRequestCounter(ctx, &sent), timeout)
	start := time.Now()
	snap, err := r.adapter.Price(fetchCtx, eventIDs)
	cancel()
	r.recordRequestCost(sent.Load(), len(eventIDs))
	r.recordFetch(ctx, opPrice, start, snap, err)
	r.ingest(ctx, snap, err == nil && len(snap.Errors) == 0)
}

func (r *runner) recordRequestCost(sent int64, events int) {
	if sent <= 0 || events == 0 {
		return
	}
	sample := float64(sent) / float64(events)
	cost := math.Float64frombits(r.requestCost.Load())
	if cost <= 0 {
		cost = sample
	} else {
		cost += requestCostWeight * (sample - cost)
	}
	r.requestCost.Store(math.Float64bits(cost))
	requestsPerEvent.WithLabelValues(r.name).Set(cost)
}

func (r *runner) recordFetch(ctx context.Context, op string, start time.Time, snap canon.Snapshot, err error) {
	took := time.Since(start)
	observability.FetchLatency.WithLabelValues(r.name, op).Observe(took.Seconds())
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		observability.FetchErrors.WithLabelValues(r.name, op).Inc()
		r.health.recordFailure(r.now(), took, err)
		r.log.Warn("source fetch failed", "source", r.name, "op", op, "error", err)
		return
	}
	r.health.recordSuccess(r.now(), took, newestObservation(snap))
	if len(snap.Errors) > 0 {
		r.log.Debug("source fetch partially failed", "source", r.name, "op", op, "errors", len(snap.Errors), "first_error", snap.Errors[0])
	}
}

func (r *runner) ingest(ctx context.Context, snap canon.Snapshot, isCompletePoll bool) {
	if len(snap.Markets) == 0 {
		return
	}
	start := time.Now()
	changed := r.hashes.changed(snap.Markets)
	unchangedMarkets.WithLabelValues(r.name).Add(float64(len(snap.Markets) - len(changed)))
	fetchedAt := snap.FetchedAt
	if fetchedAt.IsZero() {
		fetchedAt = r.now()
	}
	published, err := r.resolver.ResolveAndPublish(ctx, r.name, fetchedAt, marketsOf(changed), polledEvents(snap.Markets, isCompletePoll))
	observability.PricesWritten.WithLabelValues(r.name).Add(float64(published))
	observability.PipelineLatency.WithLabelValues("ingest").Observe(time.Since(start).Seconds())
	if err != nil {
		r.log.Warn("resolve and publish incomplete", "source", r.name, "error", err)
	}
	if errors.Is(err, resolve.ErrNotDelivered) {
		return
	}
	r.hashes.commit(changed)
	if isCompletePoll {
		r.hashes.forgetMissing(snap.Markets)
	}
}

func marketsOf(hashed []hashedMarket) []canon.RawMarket {
	markets := make([]canon.RawMarket, len(hashed))
	for i, hm := range hashed {
		markets[i] = hm.market
	}
	return markets
}

func polledEvents(markets []canon.RawMarket, isComplete bool) []resolve.PolledEvent {
	byEvent := map[string]int{}
	var polled []resolve.PolledEvent
	for _, m := range markets {
		i, seen := byEvent[m.EventSourceID]
		if !seen {
			i = len(polled)
			byEvent[m.EventSourceID] = i
			polled = append(polled, resolve.PolledEvent{SourceEventID: m.EventSourceID, Complete: isComplete})
		}
		if m.SourceMarketID != "" {
			polled[i].SourceKeys = append(polled[i].SourceKeys, m.SourceMarketID)
		}
	}
	return polled
}

func newestObservation(snap canon.Snapshot) time.Time {
	var newest time.Time
	for _, m := range snap.Markets {
		for _, s := range m.Selections {
			if s.Price.ObservedAt.After(newest) {
				newest = s.Price.ObservedAt
			}
		}
	}
	return newest
}

func (r *runner) writeHealthPeriodically(ctx context.Context) error {
	ticker := time.NewTicker(healthWriteInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.writeHealth(ctx)
		}
	}
}

func (r *runner) writeHealth(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, healthWriteTimeout)
	defer cancel()
	sourceDegraded.WithLabelValues(r.name).Set(boolGauge(r.health.isDegraded()))
	streamLive.WithLabelValues(r.name).Set(boolGauge(r.isStreamLive.Load()))
	if err := r.rdb.HSet(ctx, redis.SourceHealthKey(r.name), r.health.redisFields(r.now())).Err(); err != nil && ctx.Err() == nil {
		r.log.Warn("source health write failed", "source", r.name, "error", err)
	}
}

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func unitRandom() float64 { return rand.Float64() } //nolint:gosec
