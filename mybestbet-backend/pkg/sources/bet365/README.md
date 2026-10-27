# bet365 adapter

Bet365 has no API. The adapter asks `browserd` (profile `bet365`) to open pages and returns the intercepted
"readit" payloads, then parses them into canonical markets. It never talks to bet365 directly.

## Pages

| Use | Page (hash route on `baseUrl`) | Captured XHR substring | WS frames |
| --- | --- | --- | --- |
| Discover | `leagueIds[<code>]` if configured, else the sport's upcoming coupon `#/AC/<class>/C1/D1002/G40/J99/Q1/F^72/` | `matchmarketscontentapi` | no |
| Price | `#/AC/<class>/C1/D8/E<FI>/F3/` (all match markets for fixture `FI`) | `matchbettingcontentapi` | yes |

Sport classes (the `B` segment): soccer `B1`, basketball `B18`, tennis `B13`, American football `B12`, esports `B151`.
`SourceEvent.SourceID` is the bare `FI` for soccer (unchanged from the football-only adapter) and `<class>-<FI>` for the
other sports (`B18-161000001`), so `Price` knows which sport's page and market table to use without state.

`leagueIds` maps a canonical league code to a competition coupon route. Without one, events are taken from the
sport's upcoming coupon and assigned a league by the `CT` competition name (`leaguesByCompetition` in `leagues.go`).
Esports and tennis competitions are matched on the full `CT` first, then on the part before ` - `
(`CS2 - IEM Cologne` -> CS2, `Dota 2 - The International` -> DOTA2). A fixture whose league belongs to another sport
than the coupon is dropped. All 72 config.yaml league codes are supported (`sportByLeague`).
The content-API paths come from the live `websiteroutingdatacontentapi/routingdata` manifest
(`/matchmarketscontentapi/soccerupcomingmatches` for `D1002` coupons, `/matchbettingcontentapi/coupon` for `D8` match pages).
browserd needs no change for the new sports: its policy is host-based (`bet365.com`).

## Readit format

- A snapshot is `F|` followed by `|`-separated sections. Each section is `KIND;KEY=VALUE;KEY=VALUE;`.
- Push frames arrive as `\x14<topic>\x01F|...` (snapshot) or `\x15<topic>\x01U|...` (delta). Messages are separated by `\b`.
  Only `F` snapshots are parsed; `U` deltas are ignored, and every price call reloads the page to get a fresh snapshot.
- Hierarchy: `EV` (fixture: `FI` id, `NA` "Home v Away" or, for US sports, "Away @ Home", `CT` competition, `BC` kickoff `yyyyMMddHHmmss` UTC)
  then `MG` (market group: `ID`, `NA`), `MA` (column: `NA`), `PA` (participant: `NA` label, `OD` fractional odds, `HA` line/handicap, `SU=1` suspended).
- Odds are fractional (`6/5`, `EVS`) and converted to decimal (`1 + n/d`, rounded to 3 dp).

## Market mapping (`markets.go`, one table per sport, keyed by the lower-cased `MG` name)

Rules come from `code.DefaultRule()`: football codes settle on regulation, everything else (ML, SPREAD, TOTAL,
SET_HCP, TOTAL_GAMES, MAP_HCP, TOTAL_MAPS, QUALIFY and the non-football props) includes overtime / extra time.

### Soccer

| Group name | Code | Shape |
| --- | --- | --- |
| Full Time Result | 1X2 | participants named home / Draw / away |
| Double Chance | DC | "Home or Draw" -> label 1X / 12 / X2 |
| Goals Over/Under, Alternative Total Goals | OU | Over/Under columns; line from `HA`, the participant name, or a leading label column |
| Both Teams to Score | BTTS | Yes / No |
| Draw No Bet | DNB | home / away |
| Asian Handicap, Alternative Asian Handicap | AH | column 1 home, column 2 away; split lines `-0.5,-1.0` averaged to -0.75; line stored as the home handicap |
| Correct Score | CS | columns home / Draw / away; away-column scores are written winner-first and flipped to home-away; "Any Other Score" -> Other |
| Half Time/Full Time | HTFT | "Home - Draw" -> 1/X |
| Half Time Result | 1H_1X2 | period 1H |
| Corners 2-Way | CORN_OU | Over/Under columns |
| To Qualify | QUALIFY | home / away; a draw participant drops the group |
| Goalscorers | P_FIRST / P_LAST / P_ANYTIME | columns First / Last / Anytime; "No Goalscorer" skipped |
| Player Shots, Player Shots on Target, Player Assists, Player Fouls Committed, Player Tackles | P_SHOTS, P_SOT, P_AST, P_FOULS, P_TACKLES | columns `N+` -> over N-0.5 |
| Player to be Booked | P_CARD | yes |

### Basketball and American football

| Group name | Code | Shape |
| --- | --- | --- |
| Game Lines | ML, SPREAD, TOTAL | a label column of team names (row order away, home) then `Spread` (`HA` = that row's handicap, stored as the home line), `Total` (`HA` = `O 221.5` / `U 221.5`) and `Money` columns |
| 1st Half, 2nd Half, 1st Quarter .. 4th Quarter | ML, SPREAD, TOTAL | same layout, period 1H / 2H / Q1..Q4 |
| Player Points, Player Rebounds, Player Assists, Player Threes Made, Player Points, Rebounds & Assists | BK_PTS, BK_REB, BK_AST, BK_3PM, BK_PRA | label column of players, Over/Under columns with `HA` line per row |
| Player Passing Yards, Player Rushing Yards, Player Receiving Yards, Player Receptions, Player Passing Touchdowns | NF_PASS_YDS, NF_RUSH_YDS, NF_REC_YDS, NF_REC, NF_PASS_TD | same player over/under layout |
| Anytime Touchdown Scorer | NF_ANYTIME_TD | yes per player |

3-way regulation lines (`Money Line 3-Way`) are not ML and are dropped.

### Tennis

| Group name | Code | Shape |
| --- | --- | --- |
| To Win Match | ML | players; home = first-listed player |
| First Set Winner | ML, period S1 | players |
| Set Handicap | SET_HCP | column per player, `HA` set handicap, home line |
| Total Games 2-Way | TOTAL_GAMES | Over/Under columns |
| Player Aces, Player Double Faults | TN_ACES, TN_DF | player over/under layout; the player is the competitor |

### Esports

| Group name | Code | Shape |
| --- | --- | --- |
| Match Winner | ML | teams |
| Map Handicap | MAP_HCP | column per team, home line |
| Total Maps | TOTAL_MAPS | Over/Under columns |
| Map 1 - Winner | ML, period M1 | teams |
| Map 1 - Player Kills | ES_KILLS, period M1 | player over/under layout |

Anything else is dropped through `DropUnknownMarket`.

## Known gaps

- Fixtures are synthesized. From this sandbox's IP the site loads in browserd (frames, cookies, left-nav and
  footer readit payloads were captured and match the format above), but no coupon content was served, so every
  coupon and match payload in `testdata/bet365` (soccer, and the `coupon_<sport>.txt` / `event_16*`..`event_19*`
  files for basketball, American football, tennis and esports) was built from the observed framing and public descriptions
  of the format. Group names, `HA` usage, column layouts, the `@` fixture naming, the non-soccer `CT` names and the
  per-sport coupon routes are unverified and must be checked against live traffic from a GB residential IP.
- Esports player kills are only mapped for map 1 (`ES_KILLS` M1). No bet365 maps 1-2 kills group is known; if live traffic shows one, map it to `canon.PeriodMaps1To2`.
- Player ids are the lower-cased hyphenated name (bet365 exposes no stable player id on these pages); team is unknown.
- Push (`Streamer`) is not implemented: `U|` deltas need a stateful merge keyed by `IT`. Price reloads the kept-open page instead.
- When browserd or bet365 is down every call fails fast with an error (`ErrBrowserUnavailable`, `ErrNoOddsPayload`
  or the browser error); the rest of the pipeline is unaffected.

## Booking (`booking.go`)

`Book` returns `sources.ErrBookingBlocked`. `www.bet365.com` answers 403 from Cloudflare ("Attention Required! | Cloudflare", checked 2026-10-26) from this network, and bet365 publishes no booking-code or share-betslip endpoint.

## Event link (`links.go`)

`EventURL` returns the homepage `https://www.bet365.com/#/HO/`: match pages use internal fixture paths the adapter does not have. Not a match page.
