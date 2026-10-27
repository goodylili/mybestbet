# predictfun

REST at `baseUrl` (`https://api.predict.fun`), every request carries `x-api-key` from `SOURCE_PREDICTFUN_API_KEY` (production returns 401 without it; the key comes from their Discord). Limit 240 requests/minute (`maxRps: 4`). Websocket `wss://ws.predict.fun/ws` (`extra.wsUrl`), same header.

## Flow

- Discover: `GET /v1/categories?status=OPEN&marketVariant=<v>&first=100` (cursor `after`) for every match variant: `SPORTS_MATCH` (club football), `SPORTS_TEAM_MATCH` (NFL, NCAAF, tennis, Valorant and other Polymarket-sourced games), `SPORTS_NBA`, `SPORTS_FIFA_WORLD_CUP`, `SPORTS_FIFA_FRIENDLIES`, `ESPORTS_LOL`, `ESPORTS_CS2`, `ESPORTS_DOTA2` (enum from the OpenAPI at `/docs`). Match slugs follow the Polymarket convention (`fl1-psg-lyo-2026-10-25`), so the league is the slug prefix; for tennis (`atp`/`wta`) the tournament is the title prefix before `:` (`European Open: Jan-Lennard Struff vs Tallon Griekspoor`).
- Teams: `variantDetails.sports.teams[]` (falls back to the top-level `teams[]`, which is not in title order). Home is the `isHome` team; football and US sports without `isHome` are skipped because their titles list the away side first ("Falcons vs. Packers"); tennis and esports (LoL sends `isHome: null`) fall back to the first listed side.
- Outcome names on team-vs-team markets do not always equal the team abbreviation: NBA outcomes use the slug codes (`NYK`, `GSW`, `NOP`, `SAS`, `PHX`) while the teams carry `NY`, `GS`, `NO`, `SA`, `PHO`; esports outcomes can use a different tag (`PVN` for `pari`) or teams carry no abbreviation at all. An outcome is matched by name, then by the slug code at the same position as the team in `variantDetails.sports.teams` (`nba-orl-nyk-...`), then by its own `outcomes[].team.abbreviation`; still unmatched markets are dropped.
- Kickoff: kickoff = `endsAt - 2h` for `SPORTS_MATCH` (production sends `startsAt` = kickoff and `endsAt` = kickoff + 2h; testnet opened categories days early with `startsAt` = listing time). Every other variant has `startsAt` = scheduled start.
- Price: `GET /v1/categories/{slug}` for the match and its satellites (football: `-halftime-result`, `-exact-score`, `-total-corners`, `-more-markets`, `-player-props`; other sports: `-more-markets`, `-player-props`; 404 skipped), then `GET /v1/markets/{id}/orderbook` per mapped open market. The book holds prices for the first outcome (`Yes`, or `outcomes[0]` on team-vs-team markets); the other side is the complement (No ask = 1 - Yes bid).
- `FormShare`, Prob = ask, liquidity = ask x size at the touch. Rules are `code.DefaultRule()` (football regulation only, QUALIFY and every other sport incl. OT/ET).

## League table (`leagueIds` overrides, value = slug prefix or `atp/<tournament>`)

| Canon | Key | Canon | Key |
| --- | --- | --- | --- |
| WC / FRIENDLY | `fifwc` / `fif` | UCL / UEL / UWCL | `ucl` / `uel` / `uwcl` |
| EPL / CHAMP / FACUP / EFLCUP | `epl` / `elc` / `efa` / `efl` | LALIGA / COPADELREY / SUPERCOPA | `lal` / `cdr` / `ssc` |
| SERIEA / COPPAITALIA | `sea` / `itc` | BUNDES / DFBPOKAL | `bun` / `dfb` |
| LIGUE1 / CDF | `fl1` / `cde` | ERED / PRIMEIRA / SUPERLIG | `ere` / `por` / `tur` |
| SPFL / JPL / SSL / GSL | `scop` / `bel1` / `sui` / `gre1` | UPL / RPL / ELITESERIEN / DSL | `ukr1` / `rus` / `nor` / `den` |
| MLS / LIGAMX / LEAGUESCUP | `mls` / `mex` / `lec` | BRA1 / BRA2 / ARG1 | `bra` / `bra2` / `arg` |
| LIBERTADORES / SUDAMERICANA | `lib` / `sud` | SPL / J1 / J2 / KLEAGUE | `spl` / `jap` / `ja2` / `kor` |
| CSL / ISL / ALEAGUE | `chi` / `isp` / `aus` | NBA / NCAAB / WNBA | `nba` / `cbb` / `wnba` |
| EUROLEAGUE / EUROCUP / NBL | `euroleague` / `bkeurocup` / `bknbl` | NFL / NCAAF | `nfl` / `cfb` |
| CS2 / DOTA2 / VALORANT / LOL | `cs2` / `dota2` / `val` / `lol` | ATP_* / WTA_* | `atp/miami open`, `atp/roland garros`, `atp/wimbledon`, `atp/australian open`, `atp/us open`, `atp/zadar`, `atp/murcia`, `atp/morelos`, `atp/asuncion`, same for `wta/` |

Prefixes come from Polymarket's `GET /sports` (predict.fun mirrors Polymarket games, `provider: POLYMARKET`); Production on 2026-10-25 listed `epl`, `efl` (tagged "EFL CUP"), `lal`, `sea`, `bun`, `fl1`, `spl`, `mls`, `nfl`, `cfb`, `nba`, `atp`, `wta`, `cs2`, `dota2`, `lol`, `val`; `fif`, `fifwc`, `lec`, `chi`, `ucl` were seen on testnet only. Also listed but without a configured league, so dropped: `acle`, `col`, `nhl`, `ufc`, `mlb`, `kbo`, `crint`, `bkcba`, `hok`. UECL and ACB have no known code and are not listed.

## Market table (`marketType`, per sport)

| PredictFun | Football | Basketball / NFL | Tennis | Esports |
| --- | --- | --- | --- | --- |
| `SPORTS_MONEYLINE` | 1X2 (Yes/No per team + `Draw`, team from `variantDetails.sports.team.abbreviation`) | ML (two-team market, side of `outcomes[0]`) | ML | ML (a BO2 3-way 2-0/draw/0-2 group is dropped) |
| `SPORTS_HALFTIME_RESULT` / `SPORTS_EXACT_SCORE` | 1H_1X2 / CS (`Home 2 - 1 Away`, `Any Other Score` -> `Other`) | | | |
| `SPORTS_TOTALS` | OU | TOTAL | | TOTAL_MAPS (`O/U 2.5 Games`) |
| `SPORTS_FIRST_HALF_TOTALS` / `SPORTS_SECOND_HALF_TOTALS` | OU period 1H / 2H | TOTAL period 1H / 2H | | |
| `SPORTS_BOTH_TEAMS_TO_SCORE` (+ `_FIRST_HALF`, `_SECOND_HALF`) | BTTS (period 1H / 2H) | | | |
| `SPORTS_SPREADS` | AH | SPREAD | | |
| `SPORTS_MAP_HANDICAP` | | | | MAP_HCP |
| `SPORTS_CHILD_MONEYLINE` | | | | ML period M1 (`gameNumber` 1 or `Map 1 Winner`; later maps dropped) |
| `SPORTS_TOTAL_CORNERS` (+ `FIRST_HALF_`, `SECOND_HALF_`) | CORN_OU (period 1H / 2H) | | | |
| `SPORTS_TEAM_TO_ADVANCE` | QUALIFY (two-team market, incl. ET/pens) | | | |
| `SPORTS_PLAYER_GOALS` / `_SHOTS` / `_SHOTS_ON_TARGET` / `_ASSISTS` | P_GOALS / P_SHOTS / P_SOT / P_AST (Over = `line`) | | | |
| `SPORTS_PLAYER_GOALS_PLUS_ASSISTS` | P_GA (0.5 line only) | | | |
| `SPORTS_ANYTIME_TOUCHDOWNS` | | NF_ANYTIME_TD (NFL only) | | |

Lines come from `variantDetails.sports.line`; when absent (esports) they are parsed from the title (`O/U 2.5`, first `(-1.5)`). A spread/handicap line belongs to `outcomes[0]` and is flipped to the home handicap. Over/Under markets listed Under first are flipped. Dropped (no canon code): team totals, first-to-score, first corner, corners odd/even, second-half result, extra time, penalty shootout, goalkeeper saves, first blood, kill/round totals and handicaps (`SPORTS_KILL_OVER_UNDER_GAME`, `SPORTS_ROUND_TOTALS`, `SPORTS_ROUND_HANDICAP`), markets without a `marketType`.

## Stream

Without an API key `Stream` returns `ErrStreamNeedsAPIKey`, whose `PollsInstead() bool` method tells the ingestor to poll REST and stop retrying the stream. With a key: one `{"method":"subscribe","requestId":n,"params":["predictOrderbook/<marketId>"]}` per market; `type:"M"` messages on `predictOrderbook/*` carry full books; the server heartbeat (`topic:"heartbeat"`) must be echoed as `{"method":"heartbeat","data":<ts>}` within 15s. Sessions resync from REST and reconnect every 5 minutes.

## Thin books

A selection is `Suspended` unless both sides of its book are quoted, the best ask has size and ask - bid is at most 0.10 share. One-sided books (e.g. a lone 0.99 ask on a player prop) and wide ones (corners O/U 13.5 asked at 0.90 over and 0.98 under) would otherwise feed junk asks into consensus.

## Known gaps

- Fixtures are production responses (`api.predict.fun`) captured on 2026-10-25, trimmed by dropping records (list pages keep a few categories with one market each, detail pages keep a few lines per market type) and otherwise unedited: football `epl-liv-bri-2026-10-25` (all satellites), `lal-bar-rea-2026-10-25`, `fl1-psg-lyo-2026-10-25`; `nfl-kc-sea-2026-10-26` (+ `-player-props`), `nba-orl-nyk-2026-10-25`, `atp-struff-grieksp-2026-10-25`, `wta-rybakin-fernand-2026-10-25`, `cs2-vit-mglz-2026-10-25`, `val-bbl1-ff3-2026-10-25`, `lol-gen-ly-2026-10-25`, `dota2-ts8-pari-2026-10-26`, their orderbooks and the websocket frames.
- The only testnet data left is `categories_SPORTS_FIFA_WORLD_CUP.json`, `category_fifwc-esp-arg-2026-07-19*.json` and their orderbooks (`orderbook_787677` to `orderbook_790691`, synthesized), kept for WC, QUALIFY and football player props: production listed no World Cup or friendlies, no `SPORTS_TEAM_TO_ADVANCE` (EFL Cup ties included) and no football `-player-props` (only NFL) on 2026-10-25.
- Production tennis titles on 2026-10-25 (European Open, Erste Bank Open, Swiss Indoors Basel, Lyon Open, Almaty Open, Ningbo Open, Japan Women's Open) were all outside the configured list; the tests map two of them through `leagueIds` overrides. Major/Miami/challenger title prefixes (`US Open`, `Roland Garros`, `Miami Open`, ...) follow Polymarket naming and are unverified; override them in `leagueIds` if they differ.
- NCAAF and NBA teams carry nicknames only (`Beavers`, `Knicks`), not the full club name.
- Not offered by the venue's market-type enum: basketball player props (BK_*), NFL yardage/reception/passing props, tennis set handicap, total games and aces/double faults, esports kills (only a game-level kill total, which is not a player prop), DC/DNB/HTFT.
- Tennis `SPORTS_SPREADS` (if they appear) are game spreads, not set handicaps, and are dropped; `SPORTS_CHILD_MONEYLINE` is only mapped for esports.
- Price issues one orderbook request per market, so a full card costs ~60 requests (~15s at 240/min); the websocket is the intended hot path.
- Player ids and teams are not exposed; `SourcePlayer.SourceID` is the player name.

## Event link (`links.go`)

`EventURL` builds `https://predict.fun/market/{slug}`. Checked 2026-10-27: answered 200 with the team names once; later requests timed out from this network.
