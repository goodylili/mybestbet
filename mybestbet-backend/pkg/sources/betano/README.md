# betano

Decimal-odds sportsbook on the Kaizen Gaming platform. Base `https://www.betano.ng`.

## Endpoints

| Step | Path |
| --- | --- |
| League list | `/api/sport/{football,basketball,tennis,american-football,esports}/?req=s,stnf,c,mb,mbl` (`data.topLeagues`, `data.regionGroups[].regions[].leagues[]`) |
| League events | `/api` + league `url` + `?req=s,stnf,c,mb,mbl` (`data.blocks[].events[]`) |
| Event markets | `/api/match-odds/{slug}/{eventId}/?req=s,stnf,c,mb,mbl` (`data.event.markets`, or legacy `data.markets`) |

The slug is the path segment before the id in the event `url` (`/odds/arsenal-everton/74510237/` -> `arsenal-everton`). Discover remembers it, with the sport it was listed under, per event id; `Price` for an event that was never discovered by this adapter instance returns an error. The remembered sport picks the market table: football types are only read on football events, so a basketball `MRES` (regulation 3-way) is dropped instead of becoming `1X2`.

Leagues (`leagues.go`) are matched by sport, region and league name (`England` / `Premier League`, `USA` / `NBA`, `ATP` / `Miami`, ...) because Kaizen league ids could not be verified. Esports codes match a whole region (`Counter-Strike`, `Dota 2`, `Valorant`, `League of Legends`). Set `leagueIds` in config (canonical code -> Betano league id) to pin a league by id instead; only codes in the default table can be pinned.

## Market mapping (`market.type`)

| Betano | Canon | Selections |
| --- | --- | --- |
| MRES | `1X2` | names `1`/`X`/`2` |
| DBLC | `DC` | `1X`/`12`/`X2` |
| HCTG | `OU` | all lines in one market; side from name prefix `Over`/`Under`, line = selection `handicap` |
| BTSC | `BTTS` | `Yes`/`No` |
| DNOB | `DNB` | `1`/`2` |
| AHRF | `AH` | `columnIndex` 0 home / 1 away; home `handicap` is the line, the away selection quotes its own handicap so the line is its negation |
| CSFT | `CS` | `2-1`/`2:1`; any non-score name -> `Other` |
| HTFT | `HTFT` | label = name (`1/X`) |
| H1RS | `1H_1X2` | `1`/`X`/`2` |
| CNOU | `CORN_OU` | as HCTG |
| PSCR | `P_FIRST` / `P_ANYTIME` / `P_LAST` | `tableLayout.rows[].title` is the player, `groupSelections[i]` follows `tableLayout.columns[i]` (First / Anytime / Last) |
| PTRC | `P_CARD` | one selection per player |

Basketball, American football, tennis and esports (all synthesized type codes, see known gaps):

| Betano | Canon | Selections |
| --- | --- | --- |
| MR12 | `ML` | `1`/`2` |
| HCOT | `SPREAD` | as AHRF |
| OUOT | `TOTAL` | as HCTG |
| H1ML / H1HC / H1OU | `ML` / `SPREAD` / `TOTAL` period `1H` | |
| S1RS | `ML` period `S1` | |
| SETH | `SET_HCP` | as AHRF |
| TGOU | `TOTAL_GAMES` | as HCTG |
| M1RS | `ML` period `M1` | |
| MAPH | `MAP_HCP` | as AHRF |
| MAPT | `TOTAL_MAPS` | as HCTG |

## Known gaps

- Every Betano domain tried (`betano.ng`, `.com`, `.de`, `.pt`, `.com.gh`, ...) answered 403 with the Cloudflare "Splash Screen" page from this network, so the fixtures in `testdata/betano` are synthesized. Their shapes follow public scrapers of the same Kaizen API (field names `topLeagues`, `regionGroups`, `blocks[].events[]`, `participants`, `startTime` in ms, `markets[].type`, `selections[].price/handicap/columnIndex`, `tableLayout`) and the prices mirror the same fixtures on BetPawa/Betway. Market type codes MRES, DBLC, HCTG, BTSC, DNOB, AHRF, H1RS, CNOU, PSCR and PTRC are taken from those scrapers (CSFT and HTFT only appear in their lists of known types, with no parsed example). The AHRF away-handicap sign, the PSCR column layout and the CS "other" wording must be checked against a live NG capture.
- Re-checked 2026-10-24: `/api/sport/basketball/` still answers 403. The multi-sport fixtures (`sport_basketball|tennis|american-football|esports.json`, `league_4xx..7xx.json`, `match_751..754*.json`) are synthesized in the same shape, with prices copied from the same events on Betway/BetPawa. The type codes MR12, HCOT, OUOT, H1ML, H1HC, H1OU, S1RS, SETH, TGOU, M1RS, MAPH and MAPT, and every league/region name outside the original ten football leagues, are placeholders that must be replaced from a live NG capture; until then unknown types are dropped through `DropUnknownMarket` (logged with the sport prefix, e.g. `basketball:XXXX`). No player props are mapped outside football.
- Production access likely needs browserd (Cloudflare bot check bound to TLS fingerprint + IP) and an NG IP.
- Player props carry only the printed name; no player id or team.

## Booking (`booking.go`)

`Book` returns `sources.ErrBookingBlocked`. Every Betano request from this network answers 403 from Cloudflare (page title "Betano Splash Screen", checked 2026-10-26 on `GET /api/sharebookingcode/getbetslip`). The site's JS shows the flow, but it could not be called or verified: `POST /api/betslip/v3/AddPartsAndSingleLineAmount {selectionIds,amount,betslip}` builds a server-side betslip, then `POST /api/sharebookingcode/betslip {payload:<betslip>,feature}` with `x-api-version: 1.1` returns `{data:{code,url}}`. Load: `GET /api/sharebookingcode/getbetslip?code=<code>`. Web link: `https://www.betano.ng/bookingcode/<code>`.

## Event link (`links.go`)

`EventURL` builds `{origin}/odds/{home}-{away}/{id}/`, the shape of the fixtures' `url` field. Unverified live (2026-10-27): Cloudflare 403 from this network.
