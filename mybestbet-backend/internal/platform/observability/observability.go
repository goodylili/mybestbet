package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	FetchLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "source_fetch_seconds",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 12),
	}, []string{"source", "op"})
	FetchErrors     = promauto.NewCounterVec(prometheus.CounterOpts{Name: "source_fetch_errors_total"}, []string{"source", "op"})
	PricesWritten   = promauto.NewCounterVec(prometheus.CounterOpts{Name: "prices_written_total"}, []string{"source"})
	PipelineLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pipeline_stage_seconds",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 16),
	}, []string{"stage"})
)

const shutdownGrace = 5 * time.Second

func Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.Handle("GET /metrics", promhttp.Handler())
	if os.Getenv("PPROF") == "1" {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	}
	return mux
}

func Serve(ctx context.Context, addr string, handler http.Handler, log *slog.Logger) error {
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("http listening", "addr", addr)
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve %s: %w", addr, err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
