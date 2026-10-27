package browserd

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	fetchDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "browserd_fetch_seconds",
		Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
	}, []string{"profile"})
	fetchFailures = promauto.NewCounterVec(prometheus.CounterOpts{Name: "browserd_fetch_failures_total"}, []string{"profile"})
)

func recordFetch(profile string, started time.Time, err error) {
	fetchDuration.WithLabelValues(profile).Observe(time.Since(started).Seconds())
	if err != nil {
		fetchFailures.WithLabelValues(profile).Inc()
	}
}
