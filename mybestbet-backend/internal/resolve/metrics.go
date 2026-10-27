package resolve

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"mybestbet/internal/platform/observability"
)

const (
	stageResolve   = "resolve"
	stageReprocess = "reprocess"
	stageDedup     = "dedup"

	dedupDone    = "done"
	dedupSkipped = "skipped"
	dedupFailed  = "failed"
)

var (
	autoLinks = promauto.NewCounterVec(prometheus.CounterOpts{Name: "resolve_auto_links_total"}, []string{"kind"})
	reviews   = promauto.NewCounterVec(prometheus.CounterOpts{Name: "resolve_review_enqueued_total"}, []string{"kind"})
	inferred  = promauto.NewCounter(prometheus.CounterOpts{Name: "resolve_fixture_opponent_inferred_total"})
	pendings  = promauto.NewCounterVec(prometheus.CounterOpts{Name: "resolve_pending_markets_total"}, []string{"kind"})
	relinks   = promauto.NewCounter(prometheus.CounterOpts{Name: "resolve_event_relinks_total"})
	merges    = promauto.NewCounter(prometheus.CounterOpts{Name: "resolve_event_merges_total"})
	purged    = promauto.NewCounterVec(prometheus.CounterOpts{Name: "resolve_event_purge_rows_total"}, []string{"table", "action"})
	dedupRuns = promauto.NewCounterVec(prometheus.CounterOpts{Name: "resolve_event_dedup_runs_total"}, []string{"outcome"})
)

func countAutoLink(kind string) { autoLinks.WithLabelValues(kind).Inc() }

func countFixtureInference() { inferred.Inc() }

func countRelink() { relinks.Inc() }

func countMerges(n int) { merges.Add(float64(n)) }

func countDedupRun(outcome string) { dedupRuns.WithLabelValues(outcome).Inc() }

func countPurge(p PurgeReport) {
	for _, c := range []struct {
		table, action string
		n             int
	}{
		{"events", "deleted", p.PurgedEvents},
		{"event_links", "moved", p.MovedLinks},
		{"event_sources", "moved", p.MovedSources},
		{"lineups", "moved", p.MovedLineups},
		{"player_match_stats", "moved", p.MovedStats},
		{"closing_prices", "moved", p.MovedClosingPrices},
		{"prices_current", "deleted", p.DroppedPrices},
		{"prices_history", "deleted", p.DroppedHistory},
		{"signals", "deleted", p.DroppedSignals},
	} {
		purged.WithLabelValues(c.table, c.action).Add(float64(c.n))
	}
}

func countReview(kind string) { reviews.WithLabelValues(kind).Inc() }

func countPending(markets []PendingMarket) {
	for _, m := range markets {
		pendings.WithLabelValues(m.Kind).Inc()
	}
}

func observeStage(stage string, start time.Time) {
	observability.PipelineLatency.WithLabelValues(stage).Observe(time.Since(start).Seconds())
}
