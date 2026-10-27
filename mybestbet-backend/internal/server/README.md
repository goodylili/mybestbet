# internal/server

The `api` binary's HTTP layer. Contract: `api/openapi.yaml` (OpenAPI 3.1). `contract_test.go` validates every handler's JSON against that spec with a small schema walker (`openapi_test.go`: `$ref`, `allOf`, `required`, types, enums, `additionalProperties`; undeclared properties fail), so any drift breaks the test.

## Data sources per route

| Route | Reads |
| --- | --- |
| `/v1/sports` | `config.yaml` leagues grouped by sport (canonical `canon.Sports` order, sports without leagues omitted) + fixture counts + `canon.MarketsFor(sport, match/player)`; `markets` adds each spec's label, kind, shape and statField, plus the periods the market is offered for in that sport (`marketinfo.go`: OU/BTTS/CORN_OU and ML/SPREAD/TOTAL/TOTAL_GAMES follow the sport's segments, 1H_1X2 is 1H only, ES_KILLS is full/M1/M1-2, everything else full game) |
| `/v1/leagues` | `config.yaml` leagues (with sidebar `group` and per-sport `order`) + fixture counts from `events` (kickoff ≥ now − 2h, scheduled/live) |
| `/v1/events` | the in-process event index (below), or `events`+`teams` when the window falls outside it; coverage (sources, and `propCount` = player-prop market keys with at least one price on the board) from the in-process coverage snapshot; badges from the signal snapshot; best main-market price from the board cache ( `1X2` for football, `ML` otherwise). Once an event has kicked off, prices observed before kickoff are dropped |
| `/v1/events/{id}` | the event from the event index (one `events` row when not indexed) + its market keys and cells from the board cache (`SMEMBERS board:event:<id>` and pipelined `HGETALL board:<key>` only on a miss). Match/team markets become tabs in `canon` registry order carrying the spec's `kind` and `shape`, each split into periods (full game, 1H, 2H, Q1–Q4, S1, M1) with lines ascending. Codes missing from the registry are dropped. ~9 ms locally |
| `/v1/events/{id}/props` | same board cells where the key has a player, joined to `players` and `internal/stats.Reader` (hit rates, vs opponent, minutes, lineup, rating distribution). Research is per row and best effort: a stats error drops that row's panel and is logged, it never fails the page |
| `/v1/markets/{key}/history` | `prices_history` filtered by `eventId` too, so the `(eventId, observedAt)` index is used. Capped at 20k points |
| `/v1/players/{id}/stats` | `internal/stats.Reader` match log; opponent names and logos from one `teams` lookup |
| `/v1/signals` | the signal snapshot (open `signals` joined to the event), filtered in memory. `min` compares `greatest(margin, ev)`. Moves get `fromPrice`/`toPrice`/`change` rebuilt from their two legs (`signal.DescribeMove`), since the table has no columns for them |
| `/v1/sources` | the sources snapshot, built every 3 s from `source:health:<name>` for every adapter registered in `pkg/sources` (wire imports `pkg/sources/all`), `fake` excluded. `type`/`note` come from `config.yaml` `sources.<name>` (`config.Profiles`), `feedGroup` is `config.Feed(name)`, `feed` is `stream` when the adapter implements `sources.Streamer`. `coverage` comes from the in-process coverage snapshot. `lastError` is a display label (`Blocked from this region`, `Rate limited`, `Timed out`, `No odds returned`, `Connection failed`, `Upstream error`); the raw text stays in the ingest logs |
| `/v1/teams/{id}` | one `teams` row; `leagueCodes` = its home league plus `team_leagues`, sorted; `sport` from the home league in `config.yaml` |
| `/v1/admin/review` | `internal/review.Service`. Bearer token = `ADMIN_TOKEN`, compared as SHA-256 digests in constant time; an empty `ADMIN_TOKEN` rejects everything |
| `/v1/auth/*`, `/v1/me*` | `internal/auth.Service` (`users`, `user_identities`, `sessions`, `email_codes` from migration 0022, Redis for rate limits, cooldowns, lockouts, OAuth state and the revoked-session cache) |

## Auth

- `guard.go` wraps every route in one place: `publicRoutes` is the explicit allow-list (`/v1/meta` and the `/v1/auth/*` routes), `adminRoutes` keep the admin token, everything else needs a user access JWT (`Authorization: Bearer` wins over the `__Host-mbb_at` cookie) and gets 401 `unauthorized` with `WWW-Authenticate: Bearer`. `TestEveryNonPublicRouteRequiresAuth` fails if a new route slips through.
- `auth.Service.Authenticate` remembers a session family it found active for 5 s in process (`activeFamilies`), so most requests skip the Redis `auth:family:<id>` read. Only active results are kept; `markRevoked` (logout, password reset, refresh reuse) drops them at once, so another replica can accept a revoked session for at most 5 s (the Redis flag itself is cached for 30 s). The stream heartbeat (`SessionActive`) always reads Redis.
- Non-GET requests that carry auth cookies, and every `/v1/auth/*` POST, need an allowed `Origin` (or `Sec-Fetch-Site: same-origin` without one), else 403 `forbidden`.
- Handlers in `auth.go` only decode, call the service and set or clear cookies; tokens never appear in JSON bodies. `/v1/stream` takes the access token's expiry as its deadline and re-checks the session family on every heartbeat, so a revoked session or an expired token ends the stream and clients refresh and reconnect.
- Sign-up codes are bound to the requesting browser by the `mbb_signup` cookie: a live code's password is never replaced, another browser can neither reissue nor verify it, and only an expired code is replaced. Google sign-in binds its state to the browser with the `mbb_oauth` cookie (sha256 of the state). Both get the `__Host-` prefix when `auth.secureCookies` is true.
- Every code failure is `invalid_code` (no `code_expired`), with a 10 per 24 h per email and purpose failure budget. Login locks one email+IP pair (IPv6 by /64) for 15 minutes after 5 failures and an email everywhere after 50 failures in an hour. Argon2 runs behind a semaphore of max(2, GOMAXPROCS) slots; waiting more than 2 s returns 503 `busy` with `Retry-After: 1`.
- Ephemeral JWT key and pepper, the dev mailer and the `http://localhost:4040` origin are only used when `APP_ENV` is exactly `dev` or `test`; any other value, including empty, needs `AUTH_JWT_PRIVATE_KEY`, `AUTH_CODE_PEPPER`, `RESEND_API_KEY`, `AUTH_EMAIL_FROM` and `WEB_ORIGIN`. `make test` runs with `APP_ENV=test`.

## Sports

- An event's `sport` comes from its league in `config.yaml` (`config.League.Sport`), not from `leagues.sport`, so the API is right even before the seed has rewritten old league rows. A league missing from config counts as football, matching the column default of migration 0011.
- `?sport=` on `/v1/leagues`, `/v1/events` and `/v1/signals` resolves to that sport's league codes and filters with `= ANY($n)`; an unknown sport is a 400.
- Props work for every player code: hit rates come from `stats.Reader.HitRates`; a code the reader does not support (`stats.ErrUnsupportedCode`) keeps the prices and drops only the research panel. The rating distribution is fetched for `P_RATING` only.

## Stream (`/v1/stream?event=`)

- One Redis pub/sub connection per process (`hub.go`, client channel of 10k messages). It subscribes to `signals.live` and `board.live` and keeps a `PSUBSCRIBE event:*`, which feeds both the board cache and the clients watching an event.
- With `event`, the first frame is `event: snapshot` with the same EventDetail as `GET /v1/events/{id}`; it is read after the client is subscribed, so later `update` frames are deltas on top of it.
- Each client has a bounded buffer (256 updates). If it overflows the client is disconnected; `EventSource` reconnects after the `retry: 2000` hint and the page refetches.
- Changes are coalesced per client for 250 ms (latest cell per market/source/side/label, latest signal per id) and sent as `event: update` with a JSON `canon.BoardUpdate`. `: ping` every 15 s.
- Without `event`, a client gets signals for every event (the signals page). With `event`, it gets that event's cells and signals.
- The response sets `Cache-Control: no-cache, no-transform`. Without `no-transform`, gzip in the Next.js dev proxy (and other proxies) buffers the stream and nothing reaches the browser.

## Search (`/v1/search`) and meta (`/v1/meta`)

The query is normalized with `internal/names` (`Team` for teams and events, `Person(...).Full()` for players, `TeamLiteral` for league names) and matched against `normalizedName` by prefix, substring or pg_trgm word similarity (`<%`, GIN `gin_trgm_ops` indexes from migration 0021). One SQL statement ranks teams, players and upcoming (scheduled/live, kickoff from now-2h) events of matching teams; leagues are matched in memory from config. Rank: prefix match +1, plus `word_similarity`; events rank 0.01 below their team. `/v1/meta` returns the binary's VCS revision (or module version) `https://t.me/<telegram.botUsername>` when set, and `auth.google` when Google sign-in is configured.


## Caches

- **Coverage snapshot** (`coverage.go`): sources per event, prop counts and per-source league/market coverage, built from the Redis board (`SMEMBERS board:event:<id>` for every scheduled/live event, then `HKEYS` per market in pipelines of 250) and never from `prices_current`, which is too large to aggregate per request. It is refreshed in the background at most every 3 minutes; requests always read the last snapshot and never wait (an empty snapshot right after start).
- **Board cache** (`boardcache.go`): decoded board cells per market key and the market keys per event. While the `event:*` subscription is up entries never expire: each publish marks the market keys it carries as dirty (and the event's key list when a key is new), and every 250 ms the board feed tick reloads the dirty hashes in pipelines of 250 before it computes the board stream, so reads are served from memory and lag a change by about one tick. Entries are also reloaded after 1 minute and dropped after 10 idle minutes. While the subscription is down entries live 15 s.
- **Event index** (`eventindex.go`): every event with kickoff from now − 8 h to now + 38 d, with sports and logos, reloaded every 15 s; each reload also warms the main-market board entries. `/v1/events` windows inside it and `/v1/events/{id}` read it; anything else goes to Postgres.
- **Signal snapshot** (`signalcache.go`): up to 5000 open signals with players and logos, plus the per-event badges derived from them, reloaded every 2 s and within 0.5 s of a `signals.live` message.
- **Sources snapshot** (`sources.go`): the `/v1/sources` payload, reloaded every 3 s.
- **Opened events** (`priority.go`): `/v1/events/{id}`, `/props` and the event stream (on connect and every heartbeat) mark the event as open; once a second the marked ids are written with `ZADD GT` to the sorted set `ingest:priority:events` (score = expiry in unix ms, 5 minutes ahead) and expired members are trimmed. Ingest reads it to poll those events first.
- **Leagues snapshot**: `/v1/sports` and `/v1/leagues` fixture counts (scheduled/live, kickoff ≥ now − 2h) are counted from the event index on each reload, with league logos; `/v1/events/calendar` day counts come from the index when the window is inside it.
- **Settings** (`settingscache.go`): `/v1/me/settings` per user for 30 s; a PUT replaces the entry.
- Snapshots are refreshed by `keepRefreshed` (`snapshots.go`) from `RunStreamHub`; before the first load a request falls back to the direct query.

## HTTP caching and compression

- `compress.go` buffers every JSON response except `/v1/stream` and logos and, from 1 KiB, sends it as `br` (quality 4, quality 1 above 128 KiB) or `gzip` per `Accept-Encoding`, with `Vary: Accept-Encoding`. A 900 KB events list becomes ~160 KB in ~10 ms.
- `writeCachedJSON` (`respond.go`) gives the read routes a weak FNV-64 `ETag` (weak, so it holds across encodings) of the body, answers a matching `If-None-Match` with 304 and sets `Cache-Control: private, max-age=N, stale-while-revalidate=5N` (`private, no-cache` for settings). The max-ages are the `*MaxAge` constants.
- `responsecache.go` shares the finished, compressed response of the user-independent read routes (`sharedResponseTTL`: 1 s for events, one event, signals and sources, 5 s for sports, leagues and calendar) per path, query and encoding. A miss is built once (singleflight) while concurrent requests wait for it, and only 200s are kept. Auth runs before it, so every request is still authenticated.
