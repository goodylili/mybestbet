package resolve

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
)

const reprocessSweepInterval = time.Minute

type Reprocessor struct {
	resolver *Resolver
}

func NewReprocessor(r *Resolver) *Reprocessor { return &Reprocessor{resolver: r} }

func (p *Reprocessor) Run(ctx context.Context, sub *goredis.PubSub) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return p.resolver.FollowAliasUpdates(ctx, sub, p.reprocessAfter) })
	g.Go(func() error { return p.sweepPeriodically(ctx) })
	return g.Wait()
}

func (p *Reprocessor) reprocessAfter(ctx context.Context, u canon.AliasUpdate) {
	var err error
	switch u.Kind {
	case canon.AliasTeam:
		err = p.retryHeldEvents(ctx, u.Source)
	case canon.AliasEvent, canon.AliasPlayer:
		err = p.flush(ctx, pendingBucket{kind: u.Kind, source: u.Source, sourceKey: u.SourceID})
	}
	if err != nil {
		p.resolver.log.Warn("reprocess after alias update failed", "source", u.Source, "kind", u.Kind, "source_id", u.SourceID, "error", err)
	}
}

func (p *Reprocessor) sweepPeriodically(ctx context.Context) error {
	ticker := time.NewTicker(reprocessSweepInterval)
	defer ticker.Stop()
	for {
		if err := p.Sweep(ctx); err != nil {
			p.resolver.log.Warn("pending sweep failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (p *Reprocessor) Sweep(ctx context.Context) error {
	sources, err := p.resolver.pending.sourcesWithHeldEvents(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, source := range sources {
		errs = append(errs, p.retryHeldEvents(ctx, source))
	}
	buckets, err := p.resolver.pending.buckets(ctx)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, b := range buckets {
		if p.isResolvable(b) {
			errs = append(errs, p.flush(ctx, b))
		}
	}
	return errors.Join(errs...)
}

func (p *Reprocessor) isResolvable(b pendingBucket) bool {
	switch b.kind {
	case canon.AliasEvent:
		_, ok := p.resolver.cache.eventLink(b.source, b.sourceKey)
		return ok
	case canon.AliasPlayer:
		_, ok := p.resolver.cache.playerAlias(b.source, b.sourceKey)
		return ok
	}
	return false
}

func (p *Reprocessor) retryHeldEvents(ctx context.Context, source string) error {
	held, err := p.resolver.pending.heldEvents(ctx, source)
	if err != nil || len(held) == 0 {
		return err
	}
	linked, linkErr := p.resolver.LinkEvents(ctx, source, held)
	errs := []error{linkErr}
	for _, sourceEventID := range linked {
		errs = append(errs, p.flush(ctx, pendingBucket{kind: canon.AliasEvent, source: source, sourceKey: sourceEventID}))
	}
	return errors.Join(errs...)
}

func (p *Reprocessor) flush(ctx context.Context, b pendingBucket) error {
	start := time.Now()
	defer observeStage(stageReprocess, start)
	markets, err := p.resolver.pending.take(ctx, b)
	if err != nil || len(markets) == 0 {
		return err
	}
	var errs []error
	for fetchedAt, group := range groupByFetchTime(markets) {
		_, err := p.resolver.ResolveAndPublish(ctx, b.source, fetchedAt, group, nil)
		if errors.Is(err, ErrNotDelivered) {
			errs = append(errs, p.resolver.pending.store(ctx, asPending(b, fetchedAt, group)))
		}
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("flush %s %s %s: %w", b.kind, b.source, b.sourceKey, err)
	}
	return nil
}

func groupByFetchTime(markets []PendingMarket) map[time.Time][]canon.RawMarket {
	groups := map[time.Time][]canon.RawMarket{}
	for _, m := range markets {
		groups[m.FetchedAt] = append(groups[m.FetchedAt], m.Market)
	}
	return groups
}

func asPending(b pendingBucket, fetchedAt time.Time, markets []canon.RawMarket) []PendingMarket {
	pending := make([]PendingMarket, len(markets))
	for i, m := range markets {
		pending[i] = PendingMarket{Kind: b.kind, Source: b.source, SourceKey: b.sourceKey, Market: m, FetchedAt: fetchedAt}
	}
	return pending
}
