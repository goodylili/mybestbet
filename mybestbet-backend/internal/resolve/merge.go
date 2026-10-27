package resolve

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"mybestbet/internal/names"
	"mybestbet/pkg/canon"
)

const duplicateKickoffWindow = 3 * time.Hour

type MergeReport struct {
	Groups        int
	MergedEvents  int
	MovedLinks    int
	ClosedSignals int
}

type mergeTeam struct {
	id      string
	name    string
	sources []string
}

type mergeEvent struct {
	id        string
	league    string
	home      mergeTeam
	away      mergeTeam
	kickoff   time.Time
	createdAt time.Time
	links     int
	sources   []string
}

type eventMerge struct {
	duplicate string
	canonical string
	flipped   bool
}

func (r *Resolver) MergeDuplicateEvents(ctx context.Context) (MergeReport, error) {
	var (
		report MergeReport
		done   []eventMerge
	)
	err := r.store.inMergeLock(ctx, func(tx mergeTx) error {
		events, err := tx.unmergedEventsSince(ctx, r.now().Add(-eventCacheLookback))
		if err != nil {
			return err
		}
		merges, groups := planMerges(events, r.cache.isCollegeLeague)
		report.Groups = groups
		done = done[:0]
		for _, m := range merges {
			moved, closed, err := tx.merge(ctx, m)
			if err != nil {
				return err
			}
			report.MergedEvents++
			report.MovedLinks += moved
			report.ClosedSignals += closed
			done = append(done, m)
		}
		return nil
	})
	if err != nil {
		return MergeReport{}, fmt.Errorf("merge duplicate events: %w", err)
	}
	countMerges(report.MergedEvents)
	return report, r.announceMerges(ctx, done)
}

func (r *Resolver) announceMerges(ctx context.Context, merges []eventMerge) error {
	errs := make([]error, 0, len(merges))
	for _, m := range merges {
		errs = append(errs, r.announce(ctx, canon.AliasUpdate{Kind: aliasEventMerge, SourceID: m.duplicate, TargetID: m.canonical, Flipped: m.flipped}))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("announce event merges: %w", err)
	}
	return nil
}

func planMerges(events []mergeEvent, isCollege func(string) bool) ([]eventMerge, int) {
	ranked := slices.Clone(events)
	slices.SortStableFunc(ranked, func(a, b mergeEvent) int {
		if c := cmp.Compare(b.links, a.links); c != 0 {
			return c
		}
		if c := a.createdAt.Compare(b.createdAt); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	absorbed := make(map[string]bool, len(ranked))
	var (
		merges []eventMerge
		groups int
	)
	for i, canonical := range ranked {
		if absorbed[canonical.id] {
			continue
		}
		college := isCollege(canonical.league)
		grouped := false
		for _, other := range ranked[i+1:] {
			if absorbed[other.id] {
				continue
			}
			flipped, same := sameFixture(canonical, other, college)
			if !same {
				continue
			}
			absorbed[other.id] = true
			grouped = true
			merges = append(merges, eventMerge{duplicate: other.id, canonical: canonical.id, flipped: flipped})
		}
		if grouped {
			groups++
		}
	}
	return merges, groups
}

func sameFixture(a, b mergeEvent, college bool) (flipped, same bool) {
	if a.league != b.league || absDuration(a.kickoff.Sub(b.kickoff)) > duplicateKickoffWindow || sharesAny(a.sources, b.sources) {
		return false, false
	}
	if sameTeam(a.home, b.home, college) && sameTeam(a.away, b.away, college) {
		return false, true
	}
	if sameTeam(a.home, b.away, college) && sameTeam(a.away, b.home, college) {
		return true, true
	}
	return false, !college && a.kickoff.Equal(b.kickoff) && sharesOneSideOnly(a, b)
}

func sharesOneSideOnly(a, b mergeEvent) bool {
	if a.home.id == b.home.id {
		return !sharesAny(a.away.sources, b.away.sources)
	}
	return a.away.id == b.away.id && !sharesAny(a.home.sources, b.home.sources)
}

func sameTeam(a, b mergeTeam, college bool) bool {
	if a.id == b.id {
		return true
	}
	if college || sharesAny(a.sources, b.sources) {
		return false
	}
	return teamSimilarity(names.Team(a.name), names.Team(b.name), true) >= teamAutoLinkScore
}

func sharesAny(a, b []string) bool {
	return slices.ContainsFunc(a, func(s string) bool { return slices.Contains(b, s) })
}
