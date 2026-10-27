# loadtest

`cmd/loadtest` replays the recorded adapter fixtures through a running engine (and optionally the api) at a multiple of the hot-tier rate. It exits 1 if p95 is over 300 ms (`MaxInternalP95`, PRD "Performance → Proving it") or if any delta is dropped.

## What it sends

- Fixtures: every `testdata/<source>/golden/*price.json` (a `canon.Snapshot`) whose `source` is a registered adapter (`pkg/sources/all`). Each source event becomes one market list.
- Events: `-events` (default 40) synthetic events with deterministic UUIDv5 ids (`mybestbet/loadtest` namespace). Event `i` takes fixture event `i mod n` from every source. Players and teams get synthetic ids too, keyed by name, so props line up across sources. Markets with no selections, and repeats of one canonical key, are skipped. Nothing has to exist in Postgres or the resolver.
- Rate: each (source, event) pair is re-sent every `hotInterval / -rate` (hot tier 5 s from `config.yaml`, default `-rate 10`, so every 500 ms). Pairs are phase-spread over the period. Each send XADDs one `canon.Delta` per market to `odds.delta` (MAXLEN ~200k), with `FetchedAt`/`ObservedAt` set to the send time and every price moved independently by up to `-jitter` (default ±1%).
- Every market of every pair is re-priced on every send. That is the worst case: the real ingestor only sends markets whose hash changed.

## What it measures

- `redis-pubsub` (always): the tool subscribes to `event:<id>` for every synthetic event, which is what the api's SSE hub relays. A sent delta counts as delivered when a `BoardUpdate` cell arrives with the same market key, source and `observedAt` (the send stamp, unique per send, unlike the jittered raw price). Latency is XADD → cell received. The api's 250 ms SSE coalescing window is not included.
- `sse` (with `-api-url`): `-sse-events` events are also watched through `GET /v1/stream?event=`. Coalescing keeps only the newest cell, so a newer stamp also settles older sends of the same market and source. This path includes the 250 ms window.
- A delta not seen within `-drop-after` (5 s) is dropped. Late ones still count in the percentiles and show as `late`. After the replay the tool waits up to `-drop-after` for stragglers.
- The engine's Postgres flusher is not measured. Check the engine log for `flush prices` / `dropping unflushed` (CI does).

## Running it locally

Use a throwaway Redis and database. Never point it at the dev stack. Redis pub/sub is shared by every DB number on one server, so engine signals from a run on `localhost:6379/14` would reach the dev api through `signals.live`. Run a separate redis-server:

```sh
make build
redis-server --port 6390 --save '' --appendonly no &
make test-db
REDIS_URL=redis://localhost:6390/14 \
DATABASE_URL='postgres://mybestbet:mybestbet@localhost:5433/mybestbet_test?sslmode=disable' \
METRICS_ADDR=:9204 ENGINE_CONSUMER=loadtest bin/engine > engine.log 2>&1 &
make loadtest LOADTEST_REDIS_URL=redis://localhost:6390/14 LOADTEST_ARGS='-duration=60s'
```

To measure SSE too, also start `bin/api` with the same `REDIS_URL`/`DATABASE_URL` and `HTTP_ADDR=:8204`, then add `-api-url=http://localhost:8204` to `LOADTEST_ARGS`. Start the engine before the loadtest: its consumer group starts at `$`, so deltas written before it exists are never read.

Synthetic events have no `events` row, so `prices_current`, `closing_prices` and `signals` stay empty. `prices_history` has no event FK, so every run adds rows for the synthetic event ids (about 11M rows / 3.6 GB after a few 10x minutes). That makes later flushes slower, so delete them between runs:

```sql
DELETE FROM prices_history WHERE "eventId" IN (<the 40 synthetic ids>);
```

## Results (M-series laptop, 12 cores, 2026-10-24)

| Engine | Run | deltas/s offered | redis-pubsub p50 / p95 / p99 | dropped |
| --- | --- | --- | --- | --- |
| initial | `-rate 10`, ±1% | 80,646 | 3.14 s / 5.05 s / 6.53 s | 90% |
| batched shards, 40k-row price queue | `-rate 10` | 80,646 | 240 ms / 2.78 s / 3.36 s | 6% |
| + compact move history, disk spool, native Postgres 14 | `-rate 10` | 80,646 | 10 ms / 22 ms / 32 ms | 0 |

The last run passes the gate (4,838,752 deltas, 0 dropped, max 133 ms). During it 2.96M history rows overflowed the in-memory queue into the disk spool; the spool drained about 35 s after the run with no errors.

What got it there (see `internal/engine/README.md`):
- Shards drain their inbox in batches of up to 512 deltas and write each batch as one Redis pipeline (merged per event) plus one `XACK` from a per-shard writer.
- Signals coalesce per instance and are written every 100 ms as multi-row statements.
- `signal.MovePoint` is a 40-byte pointer-free value, so move history no longer dominates the heap the GC scans.
- Price persistence never touches the hot path: rows beyond the in-memory queue go to a gob segment spool on disk and are written once Postgres catches up, so Postgres throughput (about 70k history rows/s in Docker on this laptop) bounds drain time, not latency, and no history is dropped.

Earlier runs on this host were taken while it was swapping heavily (load average up to 240); the passing run was taken at load average ~12.
