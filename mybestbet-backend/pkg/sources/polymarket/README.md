# polymarket

Push source. Gamma (`baseUrl`) for structure, CLOB (`extra.clobUrl`) for prices, CLOB market websocket (`extra.wsUrl`) for streaming. No auth for reads.

## Flow

- Discover: `GET /sports` maps sport codes to their `series` id (fallback `primaryTagId`); `GET /events?series_id=..&active=true&closed=false&end_date_min=now&end_date_max=now+8d` paged by `offset` (100 per page), fetched once per sport code even when several leagues share it (ATP/WTA tournaments). Series is used instead of the tag because tennis matches are not tagged with the `atp`/`wta` primary tag. Only base match events (`<sport>-<home>-<away>-YYYY-MM-DD` slug with home/away teams) become `SourceEvent`s; the slug is the source event id.
- Sport of a match is the slug prefix (`nba-`, `cs2-`, `bkligend-`, ...), which picks the market table in `markets.go`.
- Price: one Gamma call per match with repeated `slug=` for the base event and its satellites (`-halftime-result`, `-exact-score`, `-total-corners`, `-more-markets`, `-player-props`), then `POST /books` (batched, 200 tokens) for every mapped token. `GET /book?token_id=` works too but costs one request per token.
- Every outcome is its own binary market. 1X2, 1H_1X2 and CS are grouped from the Yes token of each outcome market into one `RawMarket` whose `SourceMarketID` is the Gamma event slug. Two-way markets (totals, corners, BTTS, spreads, player props) use both tokens of one market.
- Price = `NewBookPrice(bid, ask, ask*askSize, FormShare)`; liquidity is USD at the touch. No asks means `Suspended`.

## League table (`leagues.go`, override with `leagueIds: {CODE: sportCode}` or `{CODE: "atp:Tournament"}`)

| Sport | Canon -> Polymarket sport code |
| --- | --- |
| football | WC fifwc, FRIENDLY fif, UCL ucl, UEL uel, UECL col, UWCL uwcl, EPL epl, CHAMP elc, FACUP efa, EFLCUP efl, LALIGA lal, COPADELREY cdr, SERIEA sea, COPPAITALIA itc, BUNDES bun, DFBPOKAL dfb, LIGUE1 fl1, CDF cde, ERED ere, PRIMEIRA por, SUPERLIG tur, SPFL scop, JPL bel1, MLS mls, LIGAMX mex, BRA1 bra, BRA2 bra2, ARG1 arg, SPL spl, SSL sui, GSL gre1, UPL ukr1, RPL rus, ELITESERIEN nor, DSL den, J1 jap, J2 ja2, KLEAGUE kor, CSL chi, ISL isp, ALEAGUE aus, LEAGUESCUP lec, LIBERTADORES lib, SUDAMERICANA sud, SUPERCOPA ssc |
| basketball | NBA nba, NCAAB cbb, WNBA wnba, EUROLEAGUE euroleague, EUROCUP bkeurocup, ACB bkligend, NBL bknbl |
| american-football | NFL nfl, NCAAF cfb |
| tennis | ATP_* atp, WTA_* wta; tournament = `eventMetadata.league` (fallback: title before `:`) with a trailing ` ATP`/` WTA` removed, matched case-insensitively and exactly: Miami Open, Roland Garros, Wimbledon, Australian Open, US Open, Zadar, Murcia, Morelos, Asuncion. Qualifying draws (`Miami Open, Qualification`) are not matched |
| esports | CS2 cs2, DOTA2 dota2, VALORANT val, LOL lol |

## Market tables (`sportsMarketType`, per sport)

Football:

| Polymarket | Canon | Notes |
| --- | --- | --- |
| `moneyline` | 1X2 | side from market slug suffix: home/away team abbreviation or `draw` |
| `soccer_halftime_result` | 1H_1X2 | suffix `home`/`draw`/`away` |
| `soccer_exact_score` | CS | suffix `2-1` (home-away), `any-other` -> `Other` |
| `totals` | OU | token0 Over, token1 Under, `line` |
| `total_corners` | CORN_OU | first/second half and team corners are dropped |
| `both_teams_to_score` | BTTS | |
| `spreads` | AH | `line` belongs to `outcomes[0]`; flipped when that team is away so Line is always the home handicap |
| `soccer_anytime_goalscorer` | P_ANYTIME | |
| `soccer_player_goals_plus_assists` | P_GA | only the 0.5 line (1+ G+A) |
| `soccer_player_goals` / `_shots` / `_shots_on_target` / `_assists` | P_GOALS / P_SHOTS / P_SOT / P_AST | Yes = over `line` |

Basketball and NFL (same table; period prefixes `first_half_` 1H, `second_half_` 2H, `q1_`..`q4_` Q1..Q4):

| Polymarket | Canon | Notes |
| --- | --- | --- |
| `moneyline` (+ period variants) | ML | one binary market, outcomes are team names/aliases; incl. OT; period ML ties resolve 50-50 on Polymarket |
| `spreads` (+ period variants) | SPREAD | `line` belongs to `outcomes[0]`, flipped to the home handicap |
| `totals` (+ period variants) | TOTAL | Over/Under |
| `points` / `rebounds` / `assists` / `threes` | BK_PTS / BK_REB / BK_AST / BK_3PM | token0 (Yes or Over) = over `line` |
| `passing_yards` / `rushing_yards` / `receiving_yards` / `receptions` / `passing_touchdowns` | NF_PASS_YDS / NF_RUSH_YDS / NF_REC_YDS / NF_REC / NF_PASS_TD | over/under `line` |
| `anytime_touchdowns` | NF_ANYTIME_TD | yes/no |

Tennis: `moneyline` ML, `tennis_first_set_winner` ML period S1, `tennis_set_handicap` SET_HCP, `tennis_match_totals` TOTAL_GAMES, `tennis_first_set_totals` TOTAL_GAMES period S1. Home = first-listed player (`ordering: home`).

Esports: `moneyline` ML (series), `child_moneyline` with slug suffix `-game1` ML period M1 (maps 2+ dropped), `map_handicap` MAP_HCP, `totals` TOTAL_MAPS ("Games Total").

Everything else (team totals, exact margin, race to N, odd/even, round handicaps/totals, kills per game, set totals, game spread, first TD, etc.) is dropped via `DropUnknownMarket`. Rules are `code.DefaultRule()`.

Player id is the numeric suffix of the market slug when present (football), otherwise the player name; name is the question prefix before `:` (cut at ` Total ` for `threes` questions like `Devin Vassell Total 3-Pointers (incl. OT): O/U 2.5`).

## Stream

Subscribe `{"assets_ids":[...],"type":"market"}`; send text `PING` every 10s. First frame is an array of `book` snapshots, then `price_change` objects (`price_changes[]` with `side` BUY=bid, SELL=ask, size 0 deletes the level). Each session resyncs from REST and reconnects every 5 minutes. Only changed markets are emitted.

## Thin books

A selection is `Suspended` unless both sides of its book are quoted, the best ask has size and ask - bid is at most 0.10 share. One-sided books (e.g. a lone 0.99 ask on a player prop) and wide ones (corners O/U 13.5 asked at 0.90 over and 0.98 under) would otherwise feed junk asks into consensus.

## Known gaps

- Player prop events were all closed at capture time; the props in `testdata/polymarket/match_epl-ars-eve-2026-10-24.json` are real markets from `epl-sun-bri-2026-10-10-player-props` re-slugged to the fixture match, and their books are synthesized.
- No DC/DNB/HTFT markets exist on Polymarket soccer, and no per-tie QUALIFY ("to advance") market exists on cup ties (FA Cup, Copa del Rey, EFL Cup, UCL knockouts only have 90-minute moneylines plus "reach the semifinal" futures).
- Not offered at capture: BK_PRA, TN_ACES, TN_DF, ES_KILLS (Dota/LoL `kill_over_under_game` is the game total, not a player). NCAAB (`cbb`), EuroCup, WNBA and UCL had no open matches in the 8-day window; tennis config tournaments were not in season, so the tennis golden maps the real Florence Challenger to ATP_CH_ZADAR through a `leagueIds` override.
- NBA player prop markets were not yet open for the 2026-27 regular season at capture; the BK_* markets in `testdata/polymarket/match_nba-mia-cha-2026-10-24.json` are real markets from `nba-cle-tor-2026-04-26` / `nba-nyk-sas-2026-06-03` re-slugged to the fixture match, with synthesized books. All other multi-sport fixtures (NBA, NFL incl. player props, ATP, CS2) and their books are real, trimmed to 3 markets per type.
- Older NBA data had a `first_half_spreads` market whose `line` sign disagreed with its question; `line` is trusted as-is.
- Player team is not exposed; `SourcePlayer.Team` is empty.

## Event link (`links.go`)

`EventURL` builds `https://polymarket.com/event/{slug}`. Checked 2026-10-27: opens the exact match (title "Chelsea FC vs. Manchester United FC Odds ...").
