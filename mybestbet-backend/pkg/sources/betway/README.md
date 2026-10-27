# betway

Decimal-odds sportsbook (Betway Africa feeds). Base `https://feeds-roa2.betwayafrica.com/br/_apis/sport/v1`, reachable without auth. Fixtures in `testdata/betway` are real captures from 2026-10-23 (football) and 2026-10-24 (NBA, NFL, ATP, CS2, LoL), trimmed.

Sport ids: `soccer`, `basketball`, `tennis`, `american-football`, `esports-counter-strike`, `esports-dota`, `esports-league-of-legends`, `esports-valorant` (Valorant's tree was empty on 2026-10-24, so the id is inferred from the others). `esports`, `cs2`, `dota-2` and similar answer an empty tree.

## Endpoints

| Step | Path |
| --- | --- |
| League tree | `/Feeds/RegionsAndLeagues/{sportId}?countryCode=NG` |
| Fixtures | `/BetBook/Filtered/?countryCode=NG&cultureCode=en-US&sportId=..&marketTypes=..&Skip=N&Take=50&RegionAndLeagueIds[i].regionId=..&RegionAndLeagueIds[i].leagueId=..` |
| Full event | `/MarketGroupings/MarketGroupNamesAndMarketsForEvent?eventId=..&countryCode=NG&cultureCode=en-US` |

`cultureCode` is required (400 without it). `marketTypes` is mandatory and must be a market name the sport uses; repeated values are OR-ed. Working names (2026-10-24): soccer `[Win/Draw/Win]`, basketball `[Basketball Win/Draw/Win]` (plus `Winner (Incl. Overtime)`), tennis `[Match Winner]`, american-football `[Money Line]`, esports `Winner` (CS2) / `[Winner]` (LoL). `[Money Line]`, `[Winner]` and similar return nothing for the other sports.

Betway event ids are Sportradar match ids (the same number BetKing exposes as `ExtProvIDItem`). Plain `regionIds`/`leagueIds` parameters are silently ignored; only the indexed `RegionAndLeagueIds[i]` filter works, and several leagues can be combined in one paged request (`isFinalPage` ends paging). Both responses expose `markets`, `outcomes` and `prices` as separate arrays joined by `marketId` / `outcomeId`.

## Ids

Ids are Sportradar UOF ids glued together: `marketId = eventId + uofMarketId + specifiers`, `outcomeId = originalMarketId + uofOutcomeId`. Example: `7222134418total=2.5~12` is event 72221344, market 18 (total goals), specifier `total=2.5`, outcome 12 (over). Lines are bundled under a "squashed parent" market (`7222134418`); each outcome's `originalMarketId` names its line and is used as the source market id.

## Market mapping (UOF market id)

| UOF | Canon | Outcome ids |
| --- | --- | --- |
| 1 | `1X2` | 1/2/3 |
| 10 | `DC` | 9 1X, 10 12, 11 X2 |
| 18 | `OU` | 12/13, line = outcome `handicap` |
| 29 | `BTTS` | 74/76 |
| 11 | `DNB` | 4/5 |
| 16 | `AH` | 1714/1715, outcome `handicap` is the home handicap (`(0 : 0.5)` -> -0.5) |
| 45 | `CS` | label from name (`1:0` -> `1-0`, `Other`) |
| 47 | `HTFT` | 418..434 -> `1/1`..`2/2` |
| 60 | `1H_1X2` | 1/2/3 |
| 166 | `CORN_OU` | 12/13 |
| 40 | `P_ANYTIME` | outcome id = player id |
| 38 (`goalnr=1`) | `P_FIRST` | player id |
| 39 | `P_LAST` | player id, 1716 "No Goal" skipped |
| 2 | `QUALIFY` | 4/5 |
| 63 / 85 | `DC` period `1H` / `2H` | 9/10/11 |
| 64 / 86 | `DNB` period `1H` / `2H` | 4/5 |
| 75 / 95 | `BTTS` period `1H` / `2H` | 74/76 |
| 66 / 88 | `AH` period `1H` / `2H` | 1714/1715, outcome `handicap` is the home handicap |
| 68 / 90 | `OU` period `1H` / `2H` | 12/13 |
| 1412 (`count=N`) | `P_SHOTS` | N or more, stored as over N-0.5 |
| 1413 (`count=N`) | `P_SOT` | same |
| 1415 (`count=N`) | `P_AST` | same |

Basketball, American football, tennis and esports share UOF ids that never appear on football events. An event whose outcomes include any of 186, 219, 223 or 225 is priced with the head-to-head table; otherwise the football table applies. This keeps basketball `1` (`[Basketball Win/Draw/Win]`, regulation 3-way), `11`, `18`, `47` and `60` from being read as football markets: they are dropped.

| UOF | Canon | Notes |
| --- | --- | --- |
| 219 Winner (Incl. Overtime) / [Money Line] | `ML` | 4/5 |
| 223 Handicap (Incl. Overtime) | `SPREAD` | 1714/1715, `hcp` = home handicap |
| 225 Total (Incl. Overtime) | `TOTAL` | 12/13 |
| 64 / 66 / 68 1st Half DNB / Handicap / Total | `ML` / `SPREAD` / `TOTAL` period `1H` | |
| 231 / 232 2nd Half Handicap / Total (Incl. Overtime) | `SPREAD` / `TOTAL` period `2H` | NFL; basketball 86/88/90 (2nd half, OT unclear) are dropped |
| 302 / 303 / 236 (`quarternr=N`) Quarter DNB / Handicap / Total | `ML` / `SPREAD` / `TOTAL` period `Q1`..`Q4` | |
| 186 [Match Winner] / Winner | `ML` | tennis and esports |
| 202 (`setnr=1`) 1st Set - Winner | `ML` period `S1` | |
| 188 Set Handicap | `SET_HCP` | |
| 189 Total Games | `TOTAL_GAMES` | |
| 327 Map Handicap | `MAP_HCP` | |
| 328 Total Maps | `TOTAL_MAPS` | |
| 330 (`mapnr=1`) 1st Map - Winner (Incl. Overtime) | `ML` period `M1` | |

187 Game Handicap is a games handicap and is dropped.

Prices of `0` (outcome not trading) are skipped; `isTradingActive=false` or a suspended market marks the price suspended.

Betway's alternate total and handicap ladders are often incoherent: the tails are clamped at one flat price pair, and some rungs run backwards (2026-10-26: NBA 73719408 has 226.5 to 233.5 all at 1.42/2.65, and NFL 71515950 prices Eagles -2.5 to +1.5 near even money while ML is 2.60). `ladder.go` groups each event's line markets by code and period and drops the clamped tails. It then walks outward from the market's `defaultLine` (or the most balanced rung if there is none) and drops every rung that is not strictly monotone against the last kept rung. Player markets are left alone.

## Leagues (`leagues.go`, `region/league` slugs; override with `leagueIds`)

Betway lists leagues with no current fixtures too, so most slugs were verified in the 2026-10-24 trees, including WC, FACUP, COPADELREY, COPPAITALIA, CDF, the Grand Slams (`atp/australian-open-men-singles`, `wta/wimbledon-women-singles`, ...), NBA, NCAAB/NCAAF (`usa/ncaa-regular-season`), NFL and the Euro cups. Not in the tree and therefore built from the same Sportradar slug pattern: UWCL, SUPERCOPA, LEAGUESCUP, RPL (`russia` listed only the cup), the Miami events and the four configured Challengers (e.g. `challenger/atp-challenger-zadar-croatia-men-singles`). Esports codes cover a whole sport: CS2 = every `esports-counter-strike` league, DOTA2 = `esports-dota`, LOL = `esports-league-of-legends`, VALORANT = `esports-valorant`. Doubles leagues are never mapped.

## Known gaps

- Coverage checked against the full NG catalog on 2026-10-26 (1,278 soccer, 145 basketball events): Betway NG lists no fixtures for A-League, Indian Super League or Russian Premier League (`russia` has only the cup), none for Liga ACB or NCAA basketball yet, two CS2 and no Dota 2 matches.
- Player names come as `"Last, First"` with no team.
- No per-player cards market was listed for the captured EPL events, so `P_CARD` is not mapped.
- No basketball, NFL, tennis or esports player props were listed (2026-10-24), so none are mapped.

## Booking (`booking.go`)

| Step | Call |
| --- | --- |
| Resolve legs | `MarketGroupNamesAndMarketsForEvent` (same fetch as Price), `eventMarkets`, then the matching outcome via `describeOutcome`/`matchSelection` |
| Create code | `POST https://www.betway.com.ng/appsynapse/bet-api-sr02/v1/Betting/BookABet`, headers `X-Brand-Id: f8a8d16a-d619-4b49-aa8c-f21211403c92` (NG, from `config.betwayafrica.com/cron/appsettings/synapse/NG`), JSON `{cultureCode:"en-US",countryCode:"NG",isSingleBet:false,isBuildABet:false,outcomes:[{outcomeId,eventId,marketId,payment:1,value:100,selected:true}]}` (`marketId` is the outcome's `marketId`, the squashed parent) |
| Response | `{bookingCode:"<code>"}` |
| Web link | `https://www.betway.com.ng/?bookingCode=<code>` (no custom app scheme) |
| Load code | `POST .../bet-api-sr02/v2/Betting/FindBookABet` `{countryCode,bookingCode,cultureCode}`, anonymous, rate-limited (`errorCode 6000359 BookABetLimitExceeded`) |

- Creating a code needs a logged-in Betway account: the site only calls BookABet when logged in (otherwise it opens the login modal) and the API answers 401 anonymously (checked 2026-10-26). `Book` maps 401 to `sources.ErrBookingUnsupported`, so no Betway code can be made without logging in.
- `testdata/betway/booking_response.json` is a real `BookABet` response body; it was answered by the horse-racing service (`/appsynapse/bet-api`), whose code does not load on the sportsbook. It is used only for the response shape.

## Event link (`links.go`)

`EventURL` builds `https://www.betway.com.ng/event/{sport}/{region}/{league}/{home}-{away}?eventId={id}` from the leagues map, matching the route in the site's Nuxt bundle. Unverified live (2026-10-27): betway.com.ng geo-redirects to betway.com from this network.
