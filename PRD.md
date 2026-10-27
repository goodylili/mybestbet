# Odds Aggregator PRD

Oct 23, 2026 · @Goodnessuc

## Overview

We are building one product that pulls prematch football odds from 17 sportsbooks and prediction markets into a single normalized feed. It covers the top 10 leagues, 10 core markets, and player props shown next to real player stats. On top of the feed sit arbitrage and value detection, line history, and Telegram alerts. An agent swarm builds all of it in one day, so this document is the build spec: every agent works against the contracts in it and nothing else.

### Goals

1. Every fixture in the 10 leagues shows all 10 markets from every source that prices them, no more than 15 seconds old for fixtures kicking off in the next 2 hours, and under 1 second for sources that push.
2. Player props from every source line up on one row per player, stat, and line, next to that player's last-N hit rate.
3. Arbs and +EV prices surface within one refresh cycle, and they reach Telegram.
4. Every price we have ever seen for a market is kept, so movement and closing line value can be charted.
5. Adding a new source means writing one adapter and nothing else.

### Non-goals for v1

- Live or in-play odds. The schema leaves room for them, but no adapter fetches them.
- Placing bets or trades. We link out and never execute.
- Sports other than football. Basketball adapters exist in the reference, and they stay switched off.
- Accounts, payments, and monetization.
- Model-driven projections. Fair prices come from de-vigged market consensus, not our own model.

### Users

Mixed and global: casual bettors comparing a price, sharp bettors hunting arbs, and prediction market traders checking where Polymarket, Kalshi, or Bayse sit against the books. The UI defaults to the simple view and lets dense data sit one click away.

### Done means

| Check | Target |
| --- | --- |
| Sources live | 17 of 17 returning fixtures, Bet365 allowed to degrade |
| Fixture match rate | 98% of fixtures matched across sources automatically |
| Player match rate | 95% automatic, the rest in a review queue |
| Refresh latency | p95 under 15 s from source change to UI in the hot tier, under 300 ms inside our pipeline |
| Alert latency | under 10 s from detection to Telegram |
| Test coverage | every adapter passes its fixture tests in CI |

## Scope

v1 covers 17 sources, 10 leagues, 10 match markets, and 12 player prop types, all prematch.

### Sources

| Source | Kind | Price form | Access | Props |
| --- | --- | --- | --- | --- |
| Polymarket | Prediction market | Share price 0 to 1 | Public REST | Some |
| Kalshi | Prediction market | Cents 1 to 99 | Public REST | Few |
| PredictFun | Prediction market (BNB Chain) | Share price 0 to 1 | REST, API key | Some |
| Bayse | Prediction market | Naira 0 to 100 | REST + WS, HMAC key | Yes |
| Bet365 | Sportsbook | Decimal | Headless browser | Yes |
| PrizePicks | DFS pick'em | Line only, fixed payout | Board API, US session | Yes |
| SportyBet | Sportsbook | Decimal | JSON API | Yes |
| football.com | Sportsbook (SportyBet stack) | Decimal | JSON API | Yes |
| 1xBet | Sportsbook | Decimal | JSON API | Yes |
| 22Bet | Sportsbook (1xBet stack) | Decimal | JSON API | Yes |
| Paripesa | Sportsbook (1xBet stack) | Decimal | JSON API | Yes |
| BetWinner | Sportsbook (1xBet stack) | Decimal | JSON API | Yes |
| Bet9ja | Sportsbook | Decimal | JSON API | Yes |
| BetKing | Sportsbook | Decimal | JSON API (CDN) | Yes |
| BetPawa | Sportsbook | Decimal | JSON API, brand headers | Some |
| Betano | Sportsbook | Decimal | JSON API | Yes |
| Betway | Sportsbook | Decimal | JSON API | Yes |

### Leagues

You had no preference, so this is the default set. Saudi Pro is in because Bayse runs a lot of markets there.

| Code | League | Country |
| --- | --- | --- |
| EPL | Premier League | England |
| LALIGA | La Liga | Spain |
| SERIEA | Serie A | Italy |
| BUNDES | Bundesliga | Germany |
| LIGUE1 | Ligue 1 | France |
| ERED | Eredivisie | Netherlands |
| PRIMEIRA | Primeira Liga | Portugal |
| CHAMP | Championship | England |
| SPL | Saudi Pro League | Saudi Arabia |
| MLS | Major League Soccer | USA |

The league list lives in config, not code. Adding UCL later is a config change plus each adapter's league ID mapping.

### Match markets

All markets settle on 90 minutes plus stoppage time. Extra time and penalties never count. Any source whose rule differs is flagged on that price and excluded from arb math.

| Code | Market | Outcomes | Line |
| --- | --- | --- | --- |
| `1X2` | Full-time result | Home, Draw, Away | None |
| `DC` | Double chance | 1X, 12, X2 | None |
| `OU` | Total goals over/under | Over, Under | 0.5 to 6.5 |
| `BTTS` | Both teams to score | Yes, No | None |
| `DNB` | Draw no bet | Home, Away | None |
| `AH` | Asian handicap | Home, Away | Quarter lines, -3.0 to +3.0 |
| `CS` | Correct score | Each score plus Other | None |
| `HTFT` | Half time / full time | 9 combos | None |
| `1H_1X2` | First half result | Home, Draw, Away | None |
| `CORN_OU` | Total corners over/under | Over, Under | 6.5 to 13.5 |

The UI shows the main line plus two lines either side by default. Every line we receive is stored.

### Player props

The stat types follow Bayse's player stats tab first (Sofascore rating, goals), then the props the books and PrizePicks price most.

| Code | Prop | Shape |
| --- | --- | --- |
| `P_ANYTIME` | Anytime scorer | Yes/No |
| `P_FIRST` | First scorer | Yes/No |
| `P_LAST` | Last scorer | Yes/No |
| `P_GOALS` | Goals | Over/under line |
| `P_SHOTS` | Shots | Over/under line |
| `P_SOT` | Shots on target | Over/under line |
| `P_AST` | Assists | Over/under line |
| `P_GA` | Goal or assist | Yes/No |
| `P_CARD` | To be booked | Yes/No |
| `P_FOULS` | Fouls committed | Over/under line |
| `P_TACKLES` | Tackles | Over/under line |
| `P_PASSES` | Passes attempted | Over/under line |
| `P_SAVES` | Goalkeeper saves | Over/under line |
| `P_RATING` | Sofascore rating | Threshold, Yes/No |

Team goals over/under (each team's own goal total, full time and halves) is stored under `T_GOALS` with the team in the market key, so a flipped event link still lands on the right team.

Team stat markets that Bayse sells as buckets (total shots, team passes, total cards) are stored under team stat codes `T_SHOTS`, `T_PASSES`, `T_CARDS`. They are compared after the bucket-to-ladder conversion in the pricing section.

## Architecture

One Go monorepo builds seven binaries that talk through Redis Streams and share one Postgres database. Adapters never touch storage. They return canonical structs, and the pipeline does the rest. That single rule is what keeps 17 adapter agents from stepping on each other.

&#91;embedded content\]

### Data flow

1. The scheduler in `ingestor` runs discovery every 10 minutes and pricing on the freshness tiers in the Performance section. Push sources (Polymarket, PredictFun, Bayse, Kalshi, Bet365) hold websocket subscriptions instead of polling.
2. `browserd` keeps one page open per Bet365 coupon and streams the intercepted websocket frames back over a persistent connection. PrizePicks only uses it to warm a session.
3. The ingestor hashes each response. An unchanged response stops there. A changed one is parsed and resolved in-process by `internal/resolve` against an in-memory alias cache, so there is no network hop for matching. Unmatched items go to the review queue.
4. Resolved price deltas go to `engine` over the `odds.delta` stream. The engine holds the board and the arb index in memory, recomputes only the markets that changed, then pipelines the board to Redis, batches history into Postgres, and emits hits to `signals`.
5. `notifier` sends signals to Telegram. `api` relays per-event Redis pub/sub channels to the frontend over SSE as deltas.
6. `stats` scrapes player match stats on its own schedule and writes straight to Postgres.

### Services

| Binary | Owns | Reads | Writes |
| --- | --- | --- | --- |
| `ingestor` | Scheduler, adapter runtime, rate limits | Source APIs, `browserd` | `odds.delta` |
| `browserd` | Pool of headless Chrome via go-rod with stealth | Bet365, PrizePicks | HTTP responses only |
| `resolver` | Review decisions and reprocessing. Live matching runs inside ingestor | `review queue`, Postgres entities | `odds.delta`, review queue |
| `engine` | Board state, history, fair price, arb, EV | `odds.delta` | Redis board, Postgres history, `signals` |
| `notifier` | Telegram delivery, dedupe, throttling | `signals` | Telegram |
| `api` | REST, SSE, admin review endpoints | Redis, Postgres | Postgres (review decisions) |
| `stats` | Sofascore, FBref, FotMob scrapers | Stat sites | Postgres player stats |
| `web` (Next.js) | UI | `api` | None |

### Storage

Postgres 16 holds entities, current prices, history, and stats. History is a plain table partitioned by day with native declarative partitioning, and a row is written only when a price changes. Redis 7 holds the live board (one hash per market), the streams, rate-limit buckets, and alert dedupe keys. No Timescale, per your call.

### Repo layout

```
/cmd/{ingestor,browserd,resolver,engine,notifier,api,stats}
/internal/canon        canonical types, market codes, the only shared contract
/internal/adapter      Adapter interface, registry, shared HTTP client
/internal/adapters/<source>   one package per source, owned by one agent
/internal/resolve      matching logic
/internal/pricing      normalization, de-vig, ladders
/internal/signal       arb and EV detection
/internal/store        Postgres and Redis access
/internal/stats        stat scrapers
/testdata/<source>     recorded fixtures per source
/web                   Next.js app
/deploy                docker-compose.yml, env templates
```

## Performance

The target is a price no more than 15 seconds old on screen for any fixture kicking off within 2 hours, and under 1 second for sources that push. Inside our own pipeline, a changed price must reach Redis and the SSE stream in under 300 ms at p95. Distance to the source is now the biggest cost, so everything we control is kept off the network.

### Freshness tiers

Poll-only sources cannot be pushed faster than they are polled, and polling every book every few seconds gets us banned. The scheduler polls by kickoff distance instead.

| Tier | Fixtures | Poll interval | Worst-case age |
| --- | --- | --- | --- |
| Hot | Kickoff within 2 h | 5 s, plus or minus 20% jitter | About 7 s |
| Warm | Kickoff within 24 h | 30 s | About 35 s |
| Cold | Kickoff in 1 to 7 days | 3 min | About 3 min |
| Discovery | All leagues | 10 min | Not applicable |

On a busy Saturday with about 40 fixtures in the hot tier, that works out to roughly 8 requests a second per book. That figure is an estimate. Each source also gets a hard request ceiling in config. When the plan would exceed it, the scheduler stretches the cold tier first, then warm, and never the hot tier.

### Push sources

| Source | Stream | Fallback |
| --- | --- | --- |
| Polymarket | CLOB market websocket per token (verify channel name) | REST book poll on the hot tier |
| PredictFun | `wss://ws.predict.fun/ws` orderbook topics | REST orderbook poll |
| Bayse | Websocket orderbook and ticker feeds | REST poll |
| Kalshi | Websocket, needs an API key | REST poll until the key exists |
| Bet365 | In-page websocket frames through `browserd` | Page reload and XHR capture |

Every push connection resyncs from a REST snapshot on reconnect, and reconciles against REST every 5 minutes to catch missed messages.

### Hot path budget

| Stage | Budget at p95 |
| --- | --- |
| Hash and skip unchanged response | 1 ms |
| Decode and map to canonical | 5 ms |
| Resolve from in-memory alias cache | 1 ms |
| Engine update and arb rescan for the changed market | 2 ms |
| Redis pipeline write and publish | 3 ms |
| SSE coalescing window | 250 ms |
| Total inside our system | under 300 ms |

### Rules for the hot path

- One pooled HTTP/2 client per source host with keep-alive, brotli and gzip on, and timeouts set per tier.
- Send `If-None-Match` or `If-Modified-Since` wherever a source honours them. Otherwise hash the body with xxhash and drop it if nothing changed.
- Decode with `goccy/go-json` into typed structs. No `map[string]any` in adapter hot paths. Reuse buffers through `sync.Pool`.
- The alias cache loads from Postgres at start. Review decisions publish on `alias.updated` and every ingestor patches its cache in place.
- The engine shards events across worker goroutines by event ID. One goroutine owns each event, so the board needs no locks. A price change recomputes only its own `MarketKey`.
- Postgres writes never sit on the hot path. History and `prices_current` go into a buffered channel and flush with `COPY` every 500 ms or 5,000 rows, whichever comes first.
- SSE sends only changed cells, per event channel, coalesced every 250 ms, with a heartbeat every 15 s.
- The frontend keeps prices in a keyed store and updates single cells. Long tables are virtualized. A tick never re-renders the whole grid.
- Arb legs must be no more than 15 seconds apart, and each signal carries the age of its oldest leg.

### Proving it

A load test replays recorded fixtures at 10 times the hot-tier rate through the whole pipeline in compose. CI fails the build if internal p95 goes over 300 ms or any delta is dropped. In production, every price carries its fetch time, so the `/sources` page shows real freshness per source, not just whether the last call succeeded.

### Region

The build ships on one VPS as decided. Which region it sits in, and whether a second ingest node is added later near the slowest sources, stays open.

## Canonical model

The `canon` package is the only contract every agent shares. The core agent writes it first and freezes it. After the freeze, changes go through the integration agent only.

### Prices

Every price is stored as an implied probability between 0 and 1, plus the raw value and its form. Decimal odds, Kalshi cents, Polymarket shares, and Bayse naira all convert at the adapter boundary, so nothing downstream ever branches on source.

```go
package canon

type PriceForm string

const (
	FormDecimal PriceForm = "decimal"
	FormShare   PriceForm = "share"
	FormCents   PriceForm = "cents"
	FormNaira   PriceForm = "naira"
	FormLineOnly PriceForm = "line_only"
)

type Price struct {
	Raw        float64
	Form       PriceForm
	Prob       float64
	Bid        *float64
	Ask        *float64
	Liquidity  *float64
	Suspended  bool
	ObservedAt time.Time
}
```

`Bid`, `Ask`, and `Liquidity` are only set for order-book sources. For those, `Prob` is the ask, because that is what a buyer pays. `FormLineOnly` is PrizePicks: it has a line and no price, so `Prob` is zero and the pricing engine skips it in arb math.

### Markets and selections

```go
type MarketCode string

type Side string

const (
	SideHome  Side = "home"
	SideDraw  Side = "draw"
	SideAway  Side = "away"
	SideOver  Side = "over"
	SideUnder Side = "under"
	SideYes   Side = "yes"
	SideNo    Side = "no"
)

type MarketKey struct {
	EventID  string
	Code     MarketCode
	Line     *float64
	PlayerID string
	Period   string
}

type Selection struct {
	Side    Side
	Label   string
	Price   Price
	DeepLink string
}

type Market struct {
	Key        MarketKey
	SourceKey  string
	Selections []Selection
	Rules      SettleRule
}

type SettleRule struct {
	RegulationOnly bool
	VoidIfNoStart  bool
	Resolver       string
}
```

A `MarketKey` is the unit of comparison. Two prices from two sources compare only when their keys are equal. Correct score and HT/FT put the score or combo in `Selection.Label` with `Side` left empty.

### Snapshots

```go
type SourceEvent struct {
	SourceID    string
	LeagueHint  string
	Home        string
	Away        string
	Kickoff     time.Time
}

type SourcePlayer struct {
	SourceID string
	Name     string
	Team     string
}

type RawMarket struct {
	EventSourceID  string
	Code           MarketCode
	Line           *float64
	Player         *SourcePlayer
	Period         string
	Selections     []Selection
	Rules          SettleRule
	SourceMarketID string
}

type Snapshot struct {
	Source    string
	FetchedAt time.Time
	Events    []SourceEvent
	Markets   []RawMarket
	Errors    []error
}
```

Adapters fill `RawMarket` with source names and IDs. The resolver turns those into a `Market` with canonical IDs. Adapters never guess canonical IDs.

### The adapter interface

```go
package adapter

type Capabilities struct {
	Markets    []canon.MarketCode
	Leagues    []string
	NeedsBrowser bool
	Geo        string
	MinInterval time.Duration
}

type Adapter interface {
	Name() string
	Capabilities() Capabilities
	Discover(ctx context.Context, leagues []string) (canon.Snapshot, error)
	Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error)
}
```

`Discover` returns events only. `Price` returns markets for the given source event IDs. Each adapter registers itself in `init()` through `adapter.Register`. The runtime gives it a shared `*http.Client` with the right proxy, a per-source rate limiter, and a `browserd` client when `NeedsBrowser` is true.

### Postgres tables

| Table | Key | Holds |
| --- | --- | --- |
| `leagues` | `code` | Canonical leagues |
| `teams` | `id` | Canonical teams |
| `team_aliases` | `source, source_name` | Source name to team ID |
| `players` | `id` | Canonical players, current team |
| `player_aliases` | `source, source_id` | Source player to player ID |
| `events` | `id` | Fixture: league, home, away, kickoff |
| `event_links` | `source, source_event_id` | Source event to event ID |
| `prices_current` | `market_key, source, side, label` | Latest price |
| `prices_history` | same + `observed_at`, partitioned by day | Every change |
| `signals` | `id` | Arbs and +EV found, with legs |
| `review_queue` | `id` | Unmatched events, teams, players |
| `player_match_stats` | `player_id, event_id` | One row per player per match |

## Source adapters

Each source gets one package and one agent. Nine of the 17 are already mapped in the endpoint reference you shared. The other six come from public docs, and the endpoints marked "verify" must be confirmed against live traffic before the adapter agent writes its parser.

### Prediction markets

| Source | Discover | Price | Notes |
| --- | --- | --- | --- |
| Polymarket | Gamma `GET /sports` for league tag IDs, then `GET /events?tag_id=…&active=true&closed=false` | Gamma event markets, then CLOB `GET /book?token_id=…` for bid/ask | Base `https://gamma-api.polymarket.com`. No auth for reads. One binary market per outcome, so 1X2 is three Yes markets |
| Kalshi | `GET /series?category=Sports&tags=Soccer` to find series like `KXEPLGAME` | `GET /events?series_ticker=…&status=open&with_nested_markets=true` | Base `https://api.elections.kalshi.com/trade-api/v2`. Reads need no auth. "Football" on Kalshi means NFL, so filter by Soccer tag. Prices in cents |
| PredictFun | `GET /v1/categories` (verify) | `GET /v1/markets/{id}/orderbook` | Base `https://api.predict.fun`. API key from their Discord. 240 requests per minute. A match is a category with home, away, draw, totals, and some player markets |
| Bayse | Events by category (verify path in API reference) | Market prices plus WS orderbook and ticker | Base `https://relay.bayse.markets`. HMAC-SHA256 key auth. Prices in naira 0 to 100. Team stat markets are 3-bucket ranges. Sofascore rating markets void if the player does not start |

### Sportsbooks from the endpoint reference

| Source | Discover | Price | Notes |
| --- | --- | --- | --- |
| SportyBet | `/factsCenter/pcUpcomingEvents` (paged) | `/factsCenter/event?eventId=…` | Base `https://www.sportybet.com/api/ng`. `bizCode=10000` envelope. Markets under `data.markets` |
| football.com | Same as SportyBet | Same as SportyBet | Base `https://www.football.com/api/ng`, its own headers. Shares one parser with SportyBet |
| 1xBet family (4 brands) | `GetChampsZip`, then `GetChampZip` per league | `GetGameZip` | Params `lng=en`, `country=132`, `partner=<id>`: 1xBet 159, 22Bet 151, Paripesa 188, BetWinner 152. Query order matters. Markets in `E[]` with `G`, `T`, `P`, `C`, `CE`, `B` |
| Bet9ja | `GetSports?DISP=0`, then `GetEventsInGroupV2` | `GetEvent?EVENTID=…` | Base `https://sports.bet9ja.com/desktop/feapi`. Odds in `event.O` keyed `marketCode[@specifier]_outcomeCode`. Soccer group market ID 1 |
| BetKing | `/api/feeds/prematch/matches/custommenu/en/4/1`, then league fixtures | `/api/feeds/prematch/event/en/4/{eventId}` | Event response split across `AreaMatches[].Items[]`. Merge fragments by `ItemID` |
| BetPawa | `/events/lists/by-queries?q=…` with category 2 | `/events/{eventId}` | Headers `x-pawa-brand: betpawa-nigeria`, `x-pawa-language: en`. Prices in `row[].prices[]` |
| Betano | `/api/sport/football/` for leagues, then league pages | `/api/match-odds/{slug}/{eventId}/` | Recover slug from event URL. Markets in `data.event.markets` or legacy `data.markets` |
| Betway | `RegionsAndLeagues/soccer?countryCode=NG`, then `BetBook/Filtered` | `MarketGroupNamesAndMarketsForEvent?eventId=…` | Catalog exposes `events`, `markets`, `outcomes`, `prices` separately; join them |

The 1xBet adapter is one package with a brand table. It counts as four sources in the registry and runs under one agent.

### Browser sources

| Source | Approach | Notes |
| --- | --- | --- |
| Bet365 | `browserd` opens the football coupon and match pages, intercepts the odds XHR and websocket frames, and returns the payloads | Heaviest anti-bot of all 17. Needs a residential IP in a Bet365 geo, persistent profiles, and human-paced navigation. The pipeline must keep working when this source is down |
| PrizePicks | `browserd` warms a US session once, then the adapter calls the board API `partner-api.prizepicks.com/projections` directly with that session | JSON:API format: join `included` players and games to projections. Soccer only. Standard, Goblin, and Demon lines are stored as separate selections |

### Adapter rules

- Map each source's market IDs to canonical codes in one table per package, `markets.go`. Unknown market IDs are logged and dropped, never guessed.
- Every adapter ships with recorded fixtures in `/testdata/<source>/`: at least one discovery response and three event responses. Tests run against fixtures only.
- Respect `MinInterval`. Back off exponentially on 429 and 403, and report the source as degraded after three failures in a row.
- No adapter logs a full response body in production.

## Entity resolution

Matching is where aggregators break, so the resolver is deterministic first and fuzzy second, and it never auto-merges anything below a confidence threshold. Every decision is stored as an alias row, so a pair is matched once and reused forever.

### Teams

1. Exact lookup in `team_aliases` by source and raw name.
2. Normalize the name: lowercase, strip accents, drop tokens like FC, CF, AFC, SC, and "club", and apply a hand-kept synonym file (`Man Utd` to `manchester united`, `Inter` to `internazionale`, `PSG` to `paris saint germain`).
3. Jaro-Winkler against teams in the same league. Score 0.93 or higher auto-links and writes an alias. 0.80 to 0.93 goes to the review queue. Below 0.80 is a new unknown team in review.

The core agent seeds `teams` and `team_aliases` for all 10 leagues from the Sofascore team list before any adapter runs.

### Events

An event matches when its league matches, both teams resolve, and kickoff is within 15 minutes. If home and away are flipped but everything else matches, the event links and the resolver flips that source's sides. Two candidate events for one source event always go to review.

### Players

1. Exact lookup in `player_aliases`.
2. Restrict candidates to the two squads in that event. This step does most of the work, because it turns a search over thousands of players into one over about 50.
3. Normalize the name (accents, initials, Arabic and Portuguese name order), then match on surname plus first initial, then full-name Jaro-Winkler at 0.90.
4. Anything else goes to review with the top three candidates attached.

Squads and player IDs come from Sofascore, which also makes it the canonical player ID space. That fits Bayse, which resolves rating markets on Sofascore.

### Review queue

The admin page in `web` lists unmatched items with suggested candidates. One click links, creates a new entity, or ignores the item. A linked item reprocesses its buffered prices right away. Target: under 5% of players and under 2% of events ever reach the queue.

## Pricing engine

The engine turns every source's number into an implied probability, strips the margin to get a fair probability, and builds one consensus fair price per selection. The UI's raw/fair toggle simply switches between `Price.Raw` and the fair figure.

### Conversion

| Form | To probability |
| --- | --- |
| Decimal odds d | 1 / d |
| Polymarket or PredictFun share s | s, using the ask |
| Kalshi cents c | c / 100, using the ask |
| Bayse naira n | n / 100 |
| PrizePicks | None, line only |

### Removing the margin

For a market with outcomes i = 1..n and implied probabilities q, the overround is the sum of q minus 1. The default de-vig is the power method: find k so the powered probabilities sum to 1.

```latex
p_i = q_i^{k}, \quad \text{where } \sum_{i=1}^{n} q_i^{k} = 1
```

Solve k by bisection on \[0.5, 3\]. The power method handles favourite-longshot bias better than plain normalization on three-way markets and correct score. Two-way markets can also use multiplicative normalization, and the method is a config flag per market code so we can compare them later.

Prediction markets list each outcome as its own Yes market. The engine groups the three 1X2 Yes asks into one market and de-vigs them together. A Polymarket book where those asks sum to 1.04 has a 4% overround just like a sportsbook.

### Consensus fair price

For each selection, fair probability is the weighted median of the de-vigged probabilities from every source with a live price. Weights start at: Polymarket and Kalshi 3 when their book depth is over $1,000 at the touch (1 otherwise), Bet365 3, Betano and Betway 2, everything else 1. Weights live in config. With fewer than three sources on a selection, the engine shows no fair price and does not run +EV on it.

### Bucket markets to ladders

Bayse sells team stats as three buckets, for example shots under 24, 24 to 31, and 32 or more. After de-vigging the three buckets to p1, p2, p3, the engine writes cumulative over/under lines at each edge:

```latex
P(\text{Over } 23.5) = p_2 + p_3, \qquad P(\text{Over } 31.5) = p_3
```

Those ladder points become normal `OU` style markets under `T_SHOTS` and compare directly with any book line at 23.5 or 31.5. Book lines that fall between bucket edges are shown alongside but never used in arb math against Bayse.

### Derived markets

Double chance and draw no bet can be built from a source's 1X2 prices. The engine computes them for the fair-price view and marks them `derived`. A derived price never becomes an arb leg, because nobody can bet it at that source.

### Asian handicap and push rules

Whole lines can push, quarter lines split the stake across two half lines. v1 compares AH only on identical lines from opposite sides, and arb math uses push-aware payouts. Mixed-line AH arbs are out of scope.

## Arbitrage, value, and alerts

The engine rescans a market every time any of its prices changes, so detection latency equals refresh latency. Every signal is stored, alerted at most once, and closed when it disappears.

### Arbitrage

Take the best available price on each outcome across all sources, after fees. An arb exists when the sum of their implied probabilities is under 1:

```latex
S = \sum_{i=1}^{n} \frac{1}{d_i^{\text{best}}} < 1, \qquad \text{margin} = 1 - S, \qquad \text{stake}_i = \frac{B}{d_i \, S}
```

B is the total bankroll for the arb. The stakes give the same payout whatever happens. For prediction markets, the effective d is 1 divided by the ask plus that venue's fee per share. Fees per source sit in config, and an agent must fill them from each venue's current fee page.

A leg is dropped from the scan when any of these hold:

- It was observed more than 15 seconds before the other legs.
- It is suspended or the book shows less than $50 at the touch (prediction markets only).
- Its settle rule differs from the other legs (for example a book counting extra time).
- It is a derived price or a PrizePicks line.

Only arbs with margin of 0.5% or more become signals. That default lives in config.

### Value (+EV)

A price is +EV when the consensus fair probability says it pays more than it should:

```latex
\text{EV} = p_{\text{fair}} \cdot d - 1
```

Signals fire at EV of 3% or more, only where a fair price exists (three or more sources). For PrizePicks the engine compares the line instead of a price: if books put the same player and stat at a different line, the gap is flagged as a line discrepancy, not an EV figure.

### Line history

`prices_history` gets a row only when a selection's raw price changes, which keeps volume manageable at 17 sources. The engine also writes the closing price for each selection, defined as the last observation before kickoff. Charts in the UI read history directly. Closing line value for any past signal is the signal price against that closing price. Retention is unlimited in v1, and daily partitions make it cheap to drop old data later.

### Telegram alerts

`notifier` runs one bot. Users subscribe in a chat with `/start` and filter with commands:

| Command | Effect |
| --- | --- |
| `/arbs 1.0` | Arbs at 1.0% margin or more |
| `/ev 5` | +EV at 5% or more |
| `/league EPL LALIGA` | Limit to these leagues |
| `/props on` | Include player prop signals |
| `/move 10` | Price moves of 10% or more in implied probability within 15 minutes |
| `/mute 2h` | Pause alerts |

Each alert carries the event, market, legs with source and price, margin or EV, stake split for a 100-unit bankroll, and deep links. A Redis dedupe key per signal and chat stops repeats, and a signal re-alerts only if its margin grows by half again. No chat gets more than 30 alerts an hour.

## Player stats

Every prop row shows how often the player actually cleared that line. Stats are scraped from free sites, with Sofascore as the primary source because it covers all 10 leagues, carries the rating Bayse settles on, and gives us the canonical player IDs.

### Sources and what each supplies

| Site | Used for | Refresh |
| --- | --- | --- |
| Sofascore | Squads, lineups, player match stats (goals, assists, shots, SoT, fouls, tackles, cards, minutes), rating | Lineups 60 min before kickoff, then every 5 min. Match stats 2 h after full time |
| FotMob | Cross-check for shots, SoT, and lineups when Sofascore is missing or late | Same as Sofascore, only for gaps |
| FBref | Season aggregates and per-90 rates for the research view | Daily at 04:00 UTC |

Scrapers use the same rate-limited HTTP client and proxy pool as the adapters. Sofascore and FotMob serve JSON behind their pages, so the scrapers call those endpoints directly. FBref is HTML and uses a parser. All three get recorded fixtures and tests like any adapter.

### Stored per player per match

`player_match_stats` holds minutes, started, goals, assists, shots, shots on target, fouls, tackles, yellow, red, rating, and the source each value came from. When two sources disagree, Sofascore wins, and the disagreement is logged.

### Research views

Each prop row in the UI expands to:

- Hit rate over the last 5, 10, and 20 matches against that exact line, counting only matches where the player played 45 minutes or more.
- Home and away split.
- The same stat in past meetings with this opponent.
- Minutes trend over the last 10 matches, because a rotation risk kills most overs.
- Lineup status once Sofascore publishes it: confirmed starter, bench, or out. A confirmed bench flips a warning on every prop for that player.
- For rating markets, the player's rating distribution over the last 20 matches.

Hit rates sit next to the market price and never change it. v1 has no projection model, so the fair price on props stays the market consensus.

### Props pipeline

Adapters emit props as normal `RawMarket` rows with `Player` set. The resolver maps the player, the engine prices them like any market, and the stats join happens in the API layer on `player_id` and stat code. Yes/No props such as anytime scorer map to `P_GOALS` over 0.5 for the hit-rate lookup.

## API and frontend

The Go `api` service is the only thing the frontend talks to. It serves JSON over REST, pushes board changes over SSE, and publishes an OpenAPI 3.1 spec that the frontend agent generates its TypeScript client from. The spec is written before either side is built.

### Endpoints

| Method | Path | Returns |
| --- | --- | --- |
| GET | `/v1/leagues` | The 10 leagues with fixture counts |
| GET | `/v1/events?league=&from=&to=` | Fixtures with kickoff and source coverage |
| GET | `/v1/events/{id}` | One fixture with every market and every source price |
| GET | `/v1/events/{id}/props` | Player props grouped by player and stat, with hit rates |
| GET | `/v1/markets/{key}/history?source=` | Price history for one market |
| GET | `/v1/signals?type=arb\|ev&min=` | Open arbs and +EV, newest first |
| GET | `/v1/players/{id}/stats?last=` | Match stats for one player |
| GET | `/v1/sources` | Source health: last success, error rate, degraded flag |
| GET | `/v1/stream?event=` | SSE stream of price and signal changes |
| GET, POST | `/v1/admin/review` | Review queue list and decisions |

Admin routes sit behind a single bearer token from env. Everything else is public and read-only. Responses for `/v1/events/{id}` come from Redis and must return under 50 ms at p95.

### Frontend

Next.js with the App Router and TypeScript, Tailwind, TanStack Table for the odds grids, and Recharts for line history. Server components render the first paint, then a client hook subscribes to `/v1/stream` and patches cells in place.

| Page | What it shows |
| --- | --- |
| `/` | Today's fixtures across all leagues, best price per 1X2 outcome, arb and EV badges |
| `/league/[code]` | Fixtures for one league |
| `/event/[id]` | Tab per market. Rows are selections, columns are sources, best price highlighted. Raw/fair toggle in the header |
| `/event/[id]/props` | One row per player and stat, columns are sources, hit-rate chip, expandable research panel |
| `/signals` | Live arbs and +EV with stake calculator and links |
| `/sources` | Health board for all 17 sources |
| `/admin/review` | Matching review queue |

The display format toggle (decimal, American, implied %) is stored in a cookie. Cells that changed in the last 10 seconds flash green or red. A source that is degraded shows its column greyed with the last-seen time.

## Agent swarm plan

Thirty-one agents build this in three waves. Wave 0 freezes the contracts in about two hours, wave 1 runs 24 agents in parallel against those contracts, and wave 2 integrates and verifies. Every agent owns exactly the directories listed for it and works in its own git worktree.

### Roster

| ID | Agent | Wave | Owns | Depends on |
| --- | --- | --- | --- | --- |
| A01 | Core contracts | 0 | `internal/canon`, `internal/adapter`, config loader | None |
| A02 | Storage | 0 | `internal/store`, migrations | A01 |
| A03 | API spec | 0 | `api/openapi.yaml` | A01 |
| A04 | Infra and CI | 0 | `deploy/`, CI workflow, Makefile | None |
| A05 | Seed data | 0 | Team, alias, squad seeds from Sofascore | A02 |
| A10 | Polymarket adapter | 1 | `adapters/polymarket` | A01 |
| A11 | Kalshi adapter | 1 | `adapters/kalshi` | A01 |
| A12 | PredictFun adapter | 1 | `adapters/predictfun` | A01 |
| A13 | Bayse adapter | 1 | `adapters/bayse` | A01 |
| A14 | SportyBet and football.com | 1 | `adapters/sporty` | A01 |
| A15 | 1xBet family (4 brands) | 1 | `adapters/onexbet` | A01 |
| A16 | Bet9ja adapter | 1 | `adapters/bet9ja` | A01 |
| A17 | BetKing adapter | 1 | `adapters/betking` | A01 |
| A18 | BetPawa adapter | 1 | `adapters/betpawa` | A01 |
| A19 | Betano adapter | 1 | `adapters/betano` | A01 |
| A20 | Betway adapter | 1 | `adapters/betway` | A01 |
| A21 | Bet365 adapter | 1 | `adapters/bet365` | A01, A22 |
| A22 | browserd | 1 | `cmd/browserd` | A01 |
| A23 | PrizePicks adapter | 1 | `adapters/prizepicks` | A01, A22 |
| A24 | Ingestor runtime | 1 | `cmd/ingestor`, scheduler | A01 |
| A25 | Resolver | 1 | `internal/resolve`, `cmd/resolver` | A01, A02, A05 |
| A26 | Pricing | 1 | `internal/pricing` | A01 |
| A27 | Signals and history | 1 | `internal/signal`, `cmd/engine` | A01, A02, A26 |
| A28 | Sofascore scraper | 1 | `internal/stats/sofascore` | A02 |
| A29 | FotMob and FBref scrapers | 1 | `internal/stats/fotmob`, `internal/stats/fbref`, `cmd/stats` | A02 |
| A30 | Notifier | 1 | `cmd/notifier` | A01 |
| A31 | API server | 1 | `cmd/api` | A02, A03 |
| A32 | Web: board and event pages | 1 | `web/app/(board)` | A03 |
| A33 | Web: props, signals, sources, admin | 1 | `web/app/(tools)` | A03 |
| A40 | Integration | 2 | Merges, contract changes, end-to-end wiring | All |
| A41 | Independent QA | 2 | Test plan and findings only, writes no product code | All |

### Rules every agent follows

- Code only inside the directories you own. If you need a change in `canon`, open a request to A40 and keep working against a stub.
- Build from recorded fixtures. An adapter agent's first job is to capture real responses into `/testdata/<source>/`. If the build sandbox cannot reach the source, it asks for the fixtures to be captured on the VPS and works from the reference docs until they land.
- Definition of done: `go vet` and `golangci-lint` clean, tests passing on fixtures, a `README.md` in the package listing market mappings and known gaps.
- Every adapter agent fills its source's row in `docs/coverage.md`: which of the 10 markets and 12 prop types it really supplies.
- No agent marks its task done while tests fail.

### Order within the day

1. Hours 0 to 2: wave 0. A01 publishes `canon` and the adapter interface first, within the first hour, and tags it `contracts-v1`.
2. Hours 1 to 8: wave 1. All adapter agents start the moment `contracts-v1` is tagged. Pipeline agents work against a fake adapter that A01 ships with the contracts.
3. Hours 8 to 11: wave 2. A40 merges in a fixed order (store, pricing, resolver, engine, adapters, api, web) and runs the full pipeline on fixtures, then live.
4. Hours 11 to 12: A41 runs the verification plan against the live system. Anything it finds goes back to the owning agent.

## Engineering standards

MyBestBet is the cautionary tale, so these rules are hard gates in CI, not suggestions.

### Code

- Go 1.23, one module. `golangci-lint` with `errcheck`, `govet`, `staticcheck`, `revive`, `gosec`, and `exhaustive` on market code switches.
- Errors wrap with `%w` and context. No `panic` outside `main`.
- Every network call takes a `context.Context` with a timeout. No global HTTP client.
- Structured logs with `log/slog`, JSON in production, one `source` and `event_id` field on every adapter log line.
- Config from env plus one `config.yaml`. No secrets in the repo.
- No comments that repeat the code. Package READMEs carry the explanation.

### Tests

| Layer | Test | Gate |
| --- | --- | --- |
| Adapters | Golden tests: fixture in, expected `Snapshot` out | Required for merge |
| Pricing | Table tests for every conversion and de-vig method, property test that fair probabilities sum to 1 | Required |
| Signals | Constructed arbs and non-arbs, stale and suspended legs | Required |
| Resolver | A labelled set of 200 team names and 300 player names per league family | 98% team, 95% player precision |
| API | Contract tests against `openapi.yaml` | Required |
| End to end | All binaries in compose, fake adapters, assert a signal reaches a test Telegram chat | Required before deploy |

### Operations

- Docker Compose on one VPS: `postgres`, `redis`, the seven Go binaries, `web`, and Caddy for TLS.
- Each binary exposes `/healthz` and Prometheus metrics on `/metrics`: fetch latency, error rate, and prices written, labelled by source.
- The `/sources` page reads those metrics. A source with three straight failures is marked degraded and its prices are excluded from arbs until it recovers.
- Nightly `pg_dump` to object storage.
- Deploys are `docker compose pull && docker compose up -d` from CI on merge to `main`.

## Risks and open questions

Bet365 is the one piece most likely to miss the one-day window. The rest of the system is designed to ship without it and pick it up when it works.

### Risks

| Risk | Impact | Fallback |
| --- | --- | --- |
| Bet365 blocks the browser pool | One sharp source missing from fair prices | Ship degraded, raise the weight of Betano and Betway, keep iterating on `browserd` stealth |
| Agent sandboxes cannot reach sources, so fixtures are late | Adapter agents stall | Capture fixtures from the VPS first thing in wave 0 |
| Geo blocks (Kalshi and PrizePicks are US-facing, Nigerian books are NG-facing) | Sources return empty or 403 | Per-source egress geo in config, proxies per geo |
| Scraping breaks terms of service for Bet365, PrizePicks, and the stat sites | Accounts or IPs banned, legal exposure | Read-only, low request rates, no logins. Your call on the legal side |
| Team and player matching errors | Fake arbs | Confidence thresholds, review queue, arbs need resolved IDs on every leg |
| Settle rule mismatches (extra time, player void rules) | Fake arbs | Rules on every market, mismatched legs excluded |
| History table growth | Disk fills | Write on change only, daily partitions |

### Open questions

- [ ] Proxy provider and egress geos for each source.
- [ ] Where Telegram alerts go: one public channel, private chats, or both.
- [ ] Fee schedules for Polymarket, Kalshi, PredictFun, and Bayse, to be filled from their current fee pages.
- [ ] Exact Bayse read endpoints and whether a partner key gives higher limits.
- [ ] PredictFun API key request through their Discord.
- [ ] VPS size and region. Lagos-adjacent for the Nigerian books, or Europe for Bet365.

## Sources

- Endpoint reference for SportyBet, football.com, 1xBet family, Bet9ja, BetKing, BetPawa, Betano, Betway: the reference doc you shared.
- [Bayse player stats markets](https://bayse.markets/player-stats) and [Bayse API docs](https://docs.bayse.markets/)
- [Polymarket fetching markets guide](https://docs.polymarket.com/market-data/fetching-markets.md)
- [Kalshi API notes](https://skillselion.com/skills/starchild-ai-agent/official-skills/kalshi-api) and [Kalshi EPL series example](https://www.quicknode.com/guides/solana-development/3rd-party-integrations/kalshi-prediction-markets-with-dflow)
- [Predict.fun API guide](https://oddspapi.io/blog/?p=2854) and [predict-fun-sdk](https://docs.rs/crate/predict-fun-sdk/latest/source/CHANGELOG.md)
- [PrizePicks board API notes](https://apify.com/automation-lab/prizepicks-player-props-scraper.md)
