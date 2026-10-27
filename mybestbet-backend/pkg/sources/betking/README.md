# betking

Decimal-odds sportsbook served from the public CDN `https://sportsapicdn-desktop.betking.com` (reachable without auth; fixtures in `testdata/betking` are real captures from 2026-10-23 (football leagues) and 2026-10-24 (EFL Cup, NBA, NBL, NFL, ATP Challenger Cali), trimmed).

Sports (menu sport id): football 1, basketball 2, tennis 5, american football 16. BetKing has no esports book: the full menu (`custommenu/en/4/0`) lists 27 sports and none is esports, and ids 1..1500 were scanned (2026-10-24), so CS2/DOTA2/VALORANT/LOL are not supported.

Price fetches up to 4 events at a time: a football event is up to 1.2 MB, so a sequential batch of 25 overran the tier fetch timeout.

Not offered on the 2026-10-26 menus: Copa del Rey, Ukraine Premier League (only the cup), NCAAF (American football lists only NFL and CFL), ACB, NCAAB and any esports.

## Endpoints

| Step | Path |
| --- | --- |
| Sport menu (Sports -> Category -> Tournaments) | `/api/feeds/prematch/matches/custommenu/en/4/{sportId}` |
| League fixtures | `/api/feeds/prematch/en/4/{tournamentId}/0/0` |
| Full event | `/api/feeds/prematch/event/en/4/{eventId}` |

Discover reads one menu per requested sport and only fetches tournaments whose `ItemEventCount > 0`. Tournaments are matched by category + tournament name (trimmed, case-insensitive; BetKing pads some names with a trailing space), because tournament ids change between editions (tennis, Liga MX Apertura/Clausura). A `leagueIds` entry (canonical code -> tournament `ItemID`) pins a league by id instead; only codes in the default table can be overridden since the table carries the sport.

`AreaMatches[].SportID` gives the sport of every event fragment, so market ids are resolved per sport: football `110` is the 3-way result, while basketball reuses `110` for "1X2 - Regular Time", which is dropped (not ML).

The event response is split across `AreaMatches[].Items[]`: every UI area repeats the same item (`ItemID`) with a subset of its `OddsCollection`, and the same collection (`OddCollectionID`) can appear in several areas (for example `Total Goals` in both "Main" and "O/U 2.5"). `mergeFragments` merges by `ItemID` and keeps each collection once.

## Market mapping (`OddsTypeID`)

| BetKing | Canon | Selections |
| --- | --- | --- |
| 110 1X2 | `1X2` | OddTypeID 4/2/5 = home/draw/away |
| 146 Double Chance | `DC` | 9/10/11 = 1X/12/X2 |
| 160 Total Goals | `OU` | 12/13 over/under, line = `SpecialValue` |
| 302 GG/NG | `BTTS` | 74/76 yes/no |
| 147 Draw No Bet | `DNB` | 4/5 |
| 305 Asian Handicap | `AH` | 1714/1715, `SpecialValue` is the home handicap (`"0 : 1.5"` -> -1.5) |
| 102 Correct Score | `CS` | label from `OddName` (`1:0` -> `1-0`, `Others` -> `Other`) |
| 9881 HT/FT | `HTFT` | 418..434 -> `1/1`..`2/2` |
| 142 1st Half - 1X2 | `1H_1X2` | 4/2/5 |
| 190 Total Corners | `CORN_OU` | 12/13, one collection holds several lines |
| 9626 Anytime Scorer | `P_ANYTIME` | yes |
| 9625 First Scorer | `P_FIRST` | yes |
| 9627 Last Scorer | `P_LAST` | yes |
| 318 To Win Tie (cups) | `QUALIFY` | 4/5, settles incl. ET/pens |
| 161 / 9280 1st / 2nd Half - Total Goals | `OU` period `1H` / `2H` | 12/13 |
| 344 / 9335 1st / 2nd Half - Handicap | `AH` period `1H` / `2H` | 1714/1715 |
| 9588 / 10299 1st / 2nd Half - Double Chance | `DC` period `1H` / `2H` | |
| 9887 / 9889 1st / 2nd Half - Both Teams to Score | `BTTS` period `1H` / `2H` | |
| 9328 / 10300 1st / 2nd Half - Draw No Bet | `DNB` period `1H` / `2H` | |
| 10298 / 1018 1st / 2nd Half - Correct Score | `CS` period `1H` / `2H` | |
| 10283 / 10284 Home / Away Total Goals | `T_GOALS`, team = `Teams[ItemOrder 1 / 2].Name` | 12/13, line = `SpecialValue` |
| 10290 / 10291 1st Half - Home / Away Total Goals | `T_GOALS` period `1H` | 12/13 |
| 10302 / 10303 2nd Half - Home / Away Total Goals | `T_GOALS` period `2H` | 12/13 |

Basketball (2) and American football (16):

| BetKing | Canon | Notes |
| --- | --- | --- |
| 9300 Moneyline / 1-2 | `ML` | incl. OT, 4/5 |
| 9322 Handicap | `SPREAD` | 1714/1715, `SpecialValue` is the home handicap (`"1.5 : 0"` -> +1.5, `"0 : 4.5"` -> -4.5) |
| 9302 Total / Total Points | `TOTAL` | 12/13 |
| 344 1st Half - Handicap, 161 1st Half - Total, 9280 2nd Half - Total | `SPREAD`/`TOTAL` period `1H` (`2H` for 9280) | basketball only |
| 10414 Quarter 1 - Handicap, 10406 Quarter 1 - Total | `SPREAD`/`TOTAL` period `Q1` | basketball only |
| `[Team] Last, First Total Points (Incl. Overtime)` | `BK_PTS` | per-player market ids are dynamic (10781x), so props are recognised by the market name; team from the bracket. Rebounds/Assists use the same wording by analogy (only Points was seen live) |

Tennis (5):

| BetKing | Canon | Notes |
| --- | --- | --- |
| 9388 1-2 | `ML` | home = first-listed player |
| 9389 Set 1 - Winner | `ML` period `S1` | only odds with `SpecialValue == 1` |
| 285 Total Games | `TOTAL_GAMES` | 12/13 |

`9966 Game Handicap` is a games handicap, not a set handicap, so it is dropped; BetKing offers no set handicap. Quarter/regulation 3-way markets (`10405`, `110` in basketball) and HT/FT are dropped.

The selection ids are Sportradar UOF outcome ids. Everything else is dropped through `DropUnknownMarket`.

## Leagues

`leagues.go` holds the category/name table for all 68 non-esports codes. Seen live on 2026-10-24: UCL, UEL, EPL, CHAMP, EFLCUP, LALIGA, SERIEA, BUNDES, DFBPOKAL, LIGUE1, ERED, PRIMEIRA, SUPERLIG (`Türkiye`), SPFL, JPL, MLS, LIGAMX (`Liga MX, Apertura`), BRA1, BRA2, ARG1, SPL, SSL, GSL, UPL, RPL, ELITESERIEN, DSL, J1, KLEAGUE, CSL, ISL, ALEAGUE (`A - League`), LIBERTADORES, SUDAMERICANA, NBA, WNBA, EUROLEAGUE, ACB, NBL, NFL, NCAAF (`USA / NCAA`). Not on the menu at capture time, names follow BetKing's Betradar-short naming and need a live check: WC, FRIENDLY, UECL, UWCL, FACUP, COPADELREY, SUPERCOPA, COPPAITALIA, CDF, J2 (2026 runs a combined `J2/J3 League`, mapped to J2), LEAGUESCUP, NCAAB, EUROCUP (only `Eurocup, Women` listed), every tennis event (BetKing prints the city only: `ATP / Basel`, `Challenger / Cali`).

## Booking codes (`booking.go`)

`Book` implements `sources.Booker`. From `www.betking.com/dist/js/site.min.js` (`coupon.createCoupon`, `coupon.bookCoupon`, `SportsBookBettingApiURL = /api/sports/v1/bet/`). Booking needs no login; it only prices and stores the coupon.

| Step | Call |
| --- | --- |
| Resolve legs | the feed event (`/api/feeds/prematch/event/en/4/{id}`) per distinct event; every `MatchOdds` entry is normalised on its own through `itemMarkets` and matched to the leg. `MatchOddsID` is the selection id |
| Create coupon | `POST https://www.betking.com/api/sports/v1/bet/create/language/en` with `{"Stake":null,"Groupings":null,"Selections":[{"SelectionId":"2510403257"}]}`, answering `{"OddsErrors":{},"ResponseStatus":0,"BetCoupon":{...,"Odds":[{"SelectionId":2510403257,...}]}}`. Once in live testing it answered `ResponseStatus` 1 with no coupon and the same request succeeded right after, so creation is tried twice |
| Book | `POST .../bet/Book/{requestTransactionId}` with the `BetCoupon` as body, answering `{"ResponseStatus":1,"BookedCouponCode":"RN28X5"}`. BetKing returns the same code when the same transaction id is replayed, so the id is the first 20 hex characters of a SHA-256 of the sorted selection ids, which makes retries idempotent |
| Load (verify) | `GET .../bet/Booked/{code}/en` returns `ResponseStatus` 1 and `BookedCoupon.Odds[]` (match, market, selection, odd) |

Selections missing from the created coupon are reported as not OK. The betting base is `extra.bettingUrl` (default `https://www.betking.com/api/sports/v1/bet`). Web link: `https://m.betking.com/en-ng/sports/book-bet/{code}` (route `($locale).sports.book-bet.($bookingCode)` of the mobile app, which renders the booked selections server side). No app scheme or expiry is exposed.

Live check 2026-10-27: `RN28X5` (Chelsea v Man Utd 1), `JQ28MB` (Chelsea GG/NG NG, Bournemouth v Leeds X) and `Y21K4P` (Coventry v Sunderland O2.5, Hull v Ipswich DC X2, Chelsea CS 2:1). All loaded back through `Booked/{code}/en` with the same selections and odds, and the book-bet page contains the code and match. Fixtures: `testdata/betking/{event_1005767843,coupon_create_2510403257,coupon_book_RN28X5}.json`.

## Known gaps

- Scorer selections carry no player id (`IDPlayer` is 0), so the player source id is the name as BetKing prints it (`"Wood, Chris"`). The team is filled from the item's `Goalscorer` block.
- Player shots, shots on target, cards, assists and goal-or-assist props are not offered as per-player markets in the prematch feed (only team totals and Bet Builder combos).
- No QUALIFY outside cup ties that carry market 318; no NFL, tennis or esports player props; no set handicap; basketball player props only appear on some leagues (NBL had them, NBA did not, 2026-10-24).
- `Outcome.ScheduleStatus != 0` is treated as suspended; only status 0 was observed.

## Event link (`links.go`)

`EventURL` builds `https://m.betking.com/en-ng/sports/prematch/{id}/{home}-vs-{away}`. Checked 2026-10-27: redirects to the canonical match page (`/en-ng/sports/football/england/premier-league/{id}/chelsea-man-utd`).
