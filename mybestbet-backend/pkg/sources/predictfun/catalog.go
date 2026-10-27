package predictfun

import (
	"context"
	"slices"
	"sync"
	"time"
)

const catalogTTL = minPollInterval

var detailVariants = []string{"SPORTS_HALFTIME_RESULT", "SPORTS_EXACT_SCORE", "SPORTS_TOTAL_CORNERS", "SPORTS_PROPS"}

var catalogVariants = slices.Concat(matchVariants, detailVariants)

type catalog struct {
	mu        sync.Mutex
	fetchedAt time.Time
	bySlug    map[string]category
}

func (a *Adapter) openCatalog(ctx context.Context) (map[string]category, []error) {
	a.catalog.mu.Lock()
	defer a.catalog.mu.Unlock()
	now := a.deps.Clock()
	if a.catalog.bySlug != nil && now.Sub(a.catalog.fetchedAt) < catalogTTL {
		return a.catalog.bySlug, nil
	}
	listed, errs := a.openVariantCategories(ctx, catalogVariants)
	bySlug := make(map[string]category, len(listed))
	for _, c := range listed {
		bySlug[c.Slug] = c
	}
	a.catalog.bySlug, a.catalog.fetchedAt = bySlug, now
	return bySlug, errs
}
