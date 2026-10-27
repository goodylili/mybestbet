# betpawa

Decimal-odds sportsbook. Base `https://www.betpawa.ng/api/sportsbook/v4` with headers `x-pawa-brand: betpawa-nigeria` and `x-pawa-language: en` (set in config, applied by `Deps.Do`). The `v3` paths in the original endpoint reference now answer `404 NOT_FOUND`; `v4` answers JSON when `Accept: application/json` is sent (the site itself asks for protobuf). Fixtures in `testdata/betpawa` are real captures from 2026-10-23 (football) and 2026-10-24 (category trees, NBA, NBL, ATP Basel), trimmed.

Categories: football `2`, basketball `3`, tennis `452`. `categories/list` and a scan of category ids 1..2000 (2026-10-24) show only football, basketball, tennis and eFootball (`101`, virtual), so NFL, NCAAF and esports are not offered by BetPawa Nigeria.

Price fetches up to 4 events at a time: a football event is about 250 KB and takes about 1 s, so a sequential batch of 25 overran the tier fetch timeout.

## Endpoints

| Step | Path |
| --- | --- |
| Competition tree | `/categories/list/{categoryId}` (`withRegions[].regions[].competitions[]` with `eventCounts.upcoming`) |
| Fixtures | `/events/lists/by-queries?q=<json>` with `{"queries":[{"query":{"eventType":"UPCOMING","categories":["2"],"zones":{"competitions":[...]},"hasOdds":true},"view":{"marketTypes":["3743"]},"skip":N,"take":100}]}` |
| Full event | `/events/{eventId}` |

Discover reads the tree of every category it needs, matches competitions by region + name (ids differ per tennis edition and per Liga MX Apertura/Clausura), skips those with no upcoming events, then sends one query per category for the matched competition ids and pages with `skip` until a page is short. `leagueIds` (canonical code -> competition id) pins a code to an id. Each event carries `category.id`, so market types are resolved per category: football `3743` (1X2) also exists on basketball events, where it is the regulation 3-way and is dropped.

## Market mapping (`marketType.id`)

| BetPawa | Canon | Notes |
| --- | --- | --- |
| 3743 1X2 - FT | `1X2` | price `typeId` 3744/3745/3746 |
| 4693 Double Chance - FT | `DC` | 4694 1X, 4695 X2, 4696 12 |
| 5000 Total Score Over/Under - FT | `OU` | line from `row.specifier.total` |
| 3795 Both Teams To Score - FT | `BTTS` | 3796/3797 |
| 4703 Draw No Bet - FT | `DNB` | 4704/4705 |
| 3774 Asian Handicap - FT | `AH` | line from `row.specifier.hcp` (home handicap) |
| 28000869 Correct Score (26 Outcomes) | `CS` | label = price name (`2-1`, `Other`) |
| 4706 HT / FT | `HTFT` | label = price name (`1/X`) |
| 3668 1X2 - 1H | `1H_1X2` | 3669/3670/3671 |
| 1096783 Total Corners Over/Under - FT | `CORN_OU` | line from `row.specifier.total` |
| 4958 / 4976 Total Score Over/Under - 1H / 2H | `OU` period `1H` / `2H` | |
| 3747 / 3756 Asian Handicap - 1H / 2H | `AH` period `1H` / `2H` | |
| 4673 / 4681 Double Chance - 1H / 2H | `DC` period `1H` / `2H` | |
| 3789 / 3792 Both Teams To Score - 1H / 2H | `BTTS` period `1H` / `2H` | |
| 4697 / 4700 Draw No Bet - 1H / 2H | `DNB` period `1H` / `2H` | |
| 3819 / 4063 Correct Score - 1H / 2H | `CS` period `1H` / `2H` | label = price name |
| 129108 Anytime Goalscorer | `P_ANYTIME` | yes |
| 129110 Goalscorer nr | `P_FIRST` | only rows with `specifier.number == "1"` |
| 129109 Last Goalscorer | `P_LAST` | yes |
| 28000815 Player to score or assist | `P_GA` | yes |
| 28000816 Player to be carded | `P_CARD` | yes |
| 28000818 Player shots | `P_SHOTS` | `specifier.number = N` means N or more, stored as over N-0.5 |
| 28000819 Player shots on target | `P_SOT` | same as shots |
| 28000827 Player saves (Goalkeeper saves) | `P_SAVES` | same as shots |
| 5006 / 5003 Total Score Over/Under - FT - Home / Away Team | `T_GOALS`, team = participant 1 / 2 name | 5007/5008, 5004/5005 |
| 4964 / 4961 Total Score Over/Under - 1H - Home / Away Team | `T_GOALS` period `1H` | 4965/4966, 4962/4963 |
| 4982 / 4979 Total Score Over/Under - 2H - Home / Away Team | `T_GOALS` period `2H` | 4983/4984, 4980/4981 |

Basketball (`3`):

| BetPawa | Canon | Notes |
| --- | --- | --- |
| 4791 Moneyline - FT incl OT | `ML` | 4792/4793 |
| 4697 / 4700 Draw No Bet - 1H / 2H | `ML` period `1H` / `2H` | 2-way, push on a tied half |
| 3777 Asian Handicap - FT incl OT | `SPREAD` | `specifier.hcp` is the home handicap |
| 3747 / 3756 / 3753 / 3762 / 3768 / 3771 Asian Handicap 1H / 2H / 1Q..4Q | `SPREAD` period `1H` `2H` `Q1`..`Q4` | |
| 5009 Total Score Over/Under - FT incl OT | `TOTAL` | `specifier.total` |
| 4958 / 4976 / 4973 / 4988 / 4994 / 4997 Totals 1H / 2H / 1Q..4Q | `TOTAL` period `1H` `2H` `Q1`..`Q4` | |

Tennis (`452`):

| BetPawa | Canon | Notes |
| --- | --- | --- |
| 2043818 Moneyline - FT | `ML` | home = participant position 1 |
| 4758 Moneyline - 1S | `ML` period `S1` | |
| 4895 Total Games - Over/Under - FT | `TOTAL_GAMES` | |
| 4880 Total Games - Over/Under - 1S | `TOTAL_GAMES` period `S1` | |

`3532590 Match Handicap Games 2-way` and `4666923 Set 1 Game Handicap` are games handicaps, not set handicaps, and are dropped; BetPawa lists no set handicap. 3-way regulation lines (`3743`, `3668`, quarter 1X2s, `4724`) and HT/FT are dropped outside football.

For player markets the price `typeId` is a stable per-player id (the same player keeps it across markets), used as the player source id; `price.team` is the participant position. `typeId` 129111 ("No Goal") and 130473 ("No Last Goal") are not players and are skipped.

## Leagues (`leagues.go`, region + competition name; override with `leagueIds`)

Names seen live in the 2026-10-24 trees (competition id in brackets): UCL (12541), UEL (12546), UECL (12545), FRIENDLY `Int. Friendly Games` (12221), EPL (11965), CHAMP (12101), EFLCUP (12348), LALIGA (12039), SERIEA (12097), BUNDES (12110), DFBPOKAL (12389), LIGUE1 (12127), ERED (12125), PRIMEIRA (12124), SUPERLIG (12182), SPFL (12281), JPL (12128), MLS (12329), LIGAMX `Liga MX, Apertura` (13643), BRA1 (13234), BRA2 (13155), ARG1 (12315), SPL `Saudi Prof. League` (12194), SSL (12397), GSL `Super League 1` (12332), UPL (12752), RPL (12563), ELITESERIEN (12535), DSL (12537), J1 (12385), J2 (12579), KLEAGUE (12529), CSL (13344), ISL (12083), ALEAGUE (12195), LIBERTADORES (12542), SUDAMERICANA (12543), NBA (11971), WNBA (13501), EUROLEAGUE (12115), ACB (11966), NBL (12094). Tennis names follow the Sportradar pattern seen live (`ATP Basel, Switzerland Men Singles`, `ATP Challenger Cali, Colombia Men Singles`, `WTA Tokyo, Japan Women Singles`); Grand Slams use `Australian Open Men Singles` style (as on Betway's Sportradar tree) with the city-style variant as fallback. Not listed at capture time and therefore unverified: WC, UWCL, FACUP, COPADELREY, SUPERCOPA, COPPAITALIA, CDF, LEAGUESCUP, NCAAB, EUROCUP (only `Eurocup, Women`), LIGAMX Clausura, and all configured tennis events.

## Booking codes (`booking.go`)

`Book` implements `sources.Booker`. From the site JS (`betslips/getBookingNumberThunk`, `sportsbook.proto.postBookingNumber`); the site posts protobuf but the endpoint also takes and answers JSON. No login.

| Step | Call |
| --- | --- |
| Resolve legs | `GET /events/{eventId}` (v4) per distinct event; every price is normalised on its own through `eventMarkets` and matched to the leg, so leg mapping is the market normaliser itself. The price `id` is the selection id |
| Create | `POST /api/sportsbook/v3/booking-number` with `{"selections":{"selections":[{"type":"SINGLE","selections":[1573825980]}]}}`, one `SINGLE` entry per leg. The adapter swaps the `/v4` suffix of `baseUrl` for `/v3` |
| Response | `{"code":"NWB75OF"}` |
| Load (verify) | `GET /api/sportsbook/v3/booking-number/{code}` returns `items[].eventInfo`, `items[].selections[].market/selectionInfo` and `items[].odds.price` |

Web link: `https://www.betpawa.ng/?bookingCode={code}` (what the share sheet builds as `${origin}?bookingCode=`). `/external-prefill?selectionIds=a,b` also loads raw price ids but is not used. No app scheme was found and the response has no expiry, so `AppURL` and `ExpiresAt` are empty.

Live check 2026-10-27: `NWB75OF` (Chelsea v Man Utd 1), `PRT0EVY` (Chelsea U2.5, Coventry v Sunderland BTTS yes) and `EJX0LKQ` (Man City v Brighton DC 1X, Bournemouth v Leeds X, Hull v Ipswich AH home -0.5, plus an O17.5 leg reported as not offered). All loaded back through the load endpoint with the same selections and prices. Fixtures: `testdata/betpawa/{event_38781890,booking_NWB75OF}.json`.

## Known gaps

- Player saves (28000827) has no canonical code and is dropped.
- Team is unknown for player markets whose prices carry no `team` field (cards, shots, score-or-assist).
- No QUALIFY: the captured EFL Cup ties list no to-qualify market. No basketball or tennis player props were listed (NBA, NBL, ATP, WTA on 2026-10-24).

## Event link (`links.go`)

`EventURL` builds `{origin}/event/{id}`. Checked 2026-10-27: opens the exact match page (title "Bet on Chelsea FC - Manchester United ...").
