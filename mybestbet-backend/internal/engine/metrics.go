package engine

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var spilledPriceRows = promauto.NewCounter(prometheus.CounterOpts{Name: "engine_price_rows_spilled_total"})

var duplicateCellsDropped = promauto.NewCounterVec(prometheus.CounterOpts{Name: "engine_duplicate_cells_dropped_total"}, []string{"source"})

var misshapenCellsDropped = promauto.NewCounterVec(prometheus.CounterOpts{Name: "engine_misshapen_cells_dropped_total"}, []string{"source", "code"})

var withdrawnCells = promauto.NewCounterVec(prometheus.CounterOpts{Name: "engine_withdrawn_cells_suspended_total"}, []string{"source"})

var staleQuotesSkipped = promauto.NewCounterVec(prometheus.CounterOpts{Name: "engine_stale_quotes_skipped_total"}, []string{"source"})
