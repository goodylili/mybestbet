package ingest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	linkState     = "linked"
	unlinkedState = "unlinked"
)

var (
	discoveredEvents = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ingest_discovered_events"}, []string{"source", "state"})
	unchangedMarkets = promauto.NewCounterVec(prometheus.CounterOpts{Name: "ingest_unchanged_markets_total"}, []string{"source"})
	sourceDegraded   = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ingest_source_degraded"}, []string{"source"})
	tierEvents       = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ingest_tier_events"}, []string{"source", "tier"})
	tierInterval     = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ingest_tier_interval_seconds"}, []string{"source", "tier"})
	streamLive       = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ingest_stream_live"}, []string{"source"})
	focusedEvents    = promauto.NewGauge(prometheus.GaugeOpts{Name: "ingest_focused_events"})
	requestsPerEvent = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ingest_requests_per_event"}, []string{"source"})
)
