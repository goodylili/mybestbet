package engine

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"mybestbet/internal/platform/observability"
)

const (
	maxPendingBatches = 64
	overflowChunks    = 8192
	priceWriters      = 4
	finalFlushTimeout = 5 * time.Second
	flushTimeout      = 30 * time.Second
)

type priceFlusher struct {
	store    *store
	spool    *spool
	every    time.Duration
	maxRows  int
	log      *slog.Logger
	hasFull  chan struct{}
	overflow chan []priceRow
	stopped  chan struct{}

	mu          sync.Mutex
	pending     [][]priceRow
	pendingRows int
}

func newPriceFlusher(store *store, spool *spool, every time.Duration, maxRows int, log *slog.Logger) *priceFlusher {
	return &priceFlusher{
		store: store, spool: spool, every: every, maxRows: maxRows, log: log,
		hasFull:  make(chan struct{}, 1),
		overflow: make(chan []priceRow, overflowChunks),
		stopped:  make(chan struct{}),
	}
}

func (f *priceFlusher) queueLimit() int { return f.maxRows * maxPendingBatches }

func (f *priceFlusher) enqueue(chunks ...[]priceRow) {
	persistable := make([][]priceRow, 0, len(chunks))
	rows := 0
	for _, chunk := range chunks {
		chunk = slices.DeleteFunc(chunk, func(r priceRow) bool { return !r.isPersistable() })
		if len(chunk) > 0 {
			persistable = append(persistable, chunk)
			rows += len(chunk)
		}
	}
	if rows == 0 {
		return
	}
	f.mu.Lock()
	hasRoom := f.pendingRows+rows <= f.queueLimit()
	if hasRoom {
		f.pending = append(f.pending, persistable...)
		f.pendingRows += rows
	}
	isFull := f.pendingRows >= f.maxRows
	f.mu.Unlock()
	if !hasRoom {
		f.handToSpiller(persistable...)
	}
	if isFull || !hasRoom {
		f.wakeWriter()
	}
}

func (f *priceFlusher) handToSpiller(chunks ...[]priceRow) {
	for _, chunk := range chunks {
		select {
		case f.overflow <- chunk:
		case <-f.stopped:
			f.spill(chunk)
		}
	}
}

func (f *priceFlusher) spillUntilStopped() {
	for {
		select {
		case chunk := <-f.overflow:
			f.spill(chunk)
		case <-f.stopped:
			f.spillQueuedOverflow()
			return
		}
	}
}

func (f *priceFlusher) spillQueuedOverflow() {
	for {
		select {
		case chunk := <-f.overflow:
			f.spill(chunk)
		default:
			return
		}
	}
}

func (f *priceFlusher) spill(chunks ...[]priceRow) {
	for _, chunk := range chunks {
		if err := f.spool.append(chunk); err != nil {
			f.log.Error("dropping prices the spool could not hold", "rows", len(chunk), "err", err)
			continue
		}
		spilledPriceRows.Add(float64(len(chunk)))
	}
}

func (f *priceFlusher) wakeWriter() {
	select {
	case f.hasFull <- struct{}{}:
	default:
	}
}

func (f *priceFlusher) takeBatch() (batch []priceRow, hasMore bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	batch = make([]priceRow, 0, f.maxRows)
	for len(f.pending) > 0 && (len(batch) == 0 || len(batch)+len(f.pending[0]) <= f.maxRows) {
		batch = append(batch, f.pending[0]...)
		f.pending = f.pending[1:]
	}
	f.pendingRows -= len(batch)
	return batch, f.pendingRows >= f.maxRows
}

func (f *priceFlusher) requeue(batch []priceRow) {
	f.mu.Lock()
	hasRoom := f.pendingRows+len(batch) <= f.queueLimit()
	if hasRoom {
		f.pending = append([][]priceRow{batch}, f.pending...)
		f.pendingRows += len(batch)
	}
	f.mu.Unlock()
	if !hasRoom {
		f.handToSpiller(batch)
	}
}

func (f *priceFlusher) run(ctx context.Context) error {
	var writers, spiller sync.WaitGroup
	spiller.Go(f.spillUntilStopped)
	for range priceWriters {
		writers.Go(func() { f.writeUntilDone(ctx) })
	}
	writers.Wait()
	close(f.stopped)
	spiller.Wait()
	f.flushOnShutdown()
	if err := f.spool.close(); err != nil {
		f.log.Error("close price spool", "err", err)
	}
	return nil
}

func (f *priceFlusher) writeUntilDone(ctx context.Context) {
	ticker := time.NewTicker(f.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-f.hasFull:
		}
		for f.writeNextBatch(ctx) {
		}
		if !f.hasQueuedBatch() {
			f.drainOneSpoolSegment(ctx)
		}
	}
}

func (f *priceFlusher) hasQueuedBatch() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pendingRows >= f.maxRows
}

func (f *priceFlusher) writeNextBatch(ctx context.Context) (hasMore bool) {
	batch, hasMore := f.takeBatch()
	if len(batch) == 0 {
		return false
	}
	if hasMore {
		f.wakeWriter()
	}
	if err := f.write(ctx, batch); err != nil {
		f.requeue(batch)
		return false
	}
	return hasMore
}

func (f *priceFlusher) drainOneSpoolSegment(ctx context.Context) {
	path, ok, err := f.spool.claimOldest()
	if err != nil {
		f.log.Error("claim spool segment", "err", err)
		return
	}
	if !ok {
		return
	}
	rows, err := readSpoolSegment(path)
	if err != nil {
		f.log.Error("read spool segment", "path", path, "err", err)
	}
	isWritten := f.writeInBatches(ctx, rows)
	if err := f.spool.release(path, isWritten); err != nil {
		f.log.Error("release spool segment", "err", err)
	}
	if isWritten {
		f.wakeWriter()
	}
}

func (f *priceFlusher) writeInBatches(ctx context.Context, rows []priceRow) bool {
	for batch := range slices.Chunk(rows, f.maxRows) {
		if f.write(ctx, batch) != nil {
			return false
		}
	}
	return true
}

func (f *priceFlusher) write(ctx context.Context, batch []priceRow) error {
	writeCtx, cancel := context.WithTimeout(ctx, flushTimeout)
	defer cancel()
	if err := f.store.writePrices(writeCtx, batch); err != nil {
		f.log.Error("flush prices", "rows", len(batch), "err", err)
		return err
	}
	countWrittenPrices(batch)
	return nil
}

func (f *priceFlusher) flushOnShutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
	defer cancel()
	for batch, _ := f.takeBatch(); len(batch) > 0; batch, _ = f.takeBatch() {
		if ctx.Err() != nil || f.write(ctx, batch) != nil {
			f.spill(batch)
		}
	}
}

func countWrittenPrices(batch []priceRow) {
	bySource := map[string]int{}
	for _, row := range batch {
		bySource[row.cell.Source]++
	}
	for source, n := range bySource {
		observability.PricesWritten.WithLabelValues(source).Add(float64(n))
	}
}
