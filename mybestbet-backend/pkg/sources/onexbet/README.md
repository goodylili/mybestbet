# onexbet

One package for the 1xBet line feed, with a brand table that registers four sources:

| Registry name | Base URL | Default partner (`extra.partner`) |
| --- | --- | --- |
| onexbet | https://1xbet.ng | 159 |
| 22bet | https://22bet.ng | 151 |
| paripesa | https://paripesa.ng | 188 |
| betwinner | https://betwinner.ng | 152 |

`extra.country` defaults to 132 (Nigeria). All four brands serve the same game ids and league ids, so they share one fixture set in `testdata/onexbet/` (goldens per brand in `testdata/onexbet/golden/`).

## Endpoints (query parameters are composed in this exact order)

| Call | URL |
| --- | --- |
| Championship list | `/service-api/LineFeed/GetChampsZip?sport=<SI>&lng=en&country=<c>&partner=<p>` (only for codes resolved by name, at most once per sport per Discover) |
| Discover | `/service-api/LineFeed/GetChampZip?champ=<LI>&lng=en&country=<c>&partner=<p>` |
| Price | `/service-api/LineFeed/GetGameZip?id=<I>&lng=en&isSubGames=true&GroupEvents=true&countevents=2000&country=<c>&partner=<p>` |

Responses are `{Success, Error, Value}`. An empty championship comes back as `Value: null`.

GetChampZip also lists specials ("Home" v "Away" multi-match bets) that carry a non-empty `DI` or `TG`; only plain fixtures are returned by Discover.

## Competitions (`competitions.go`, override with `leagueIds` = championship `LI`)

Line-feed sport ids (`SI`): football 1, basketball 3, tennis 4, American football 13, esports 40 (all games share 40; the game is the championship name prefix).

- Fixed `LI`, all seen in GetChampsZip on 2026-10-24: EPL 88637, CHAMP 105759, LALIGA 127733, SERIEA 110163, BUNDES 96463, LIGUE1 12821, ERED 2018750, PRIMEIRA 3007689, SPL 16819, MLS 828065, UCL 118587, UEL 118593, UECL 2252762, FRIENDLY 125983, EFLCUP 119237, DFBPOKAL 119235, SUPERLIG 11113, SPFL 13521, JPL 28787, LIGAMX 2306111, BRA1 1268397, BRA2 57265, ARG1 119599, SSL 27695, GSL 8777, UPL 29949, RPL 225733, ELITESERIEN 1793471, DSL 8773, J1 118737, J2 30005, KLEAGUE 30467, CSL 58043, ISL 1122087, ALEAGUE 2905446, LIBERTADORES 142091, SUDAMERICANA 1528791; NBA 13589, WNBA 197289, EUROLEAGUE 139460, EUROCUP 140411, ACB 110509, NBL 220263; NFL 68315, NCAAF 93369.
- Exact championship name (not listed on capture day, names follow the live pattern `ATP. Almaty`, `WTA. Osaka`, `Challenger. Cali`, `England. League Cup`): WC "FIFA World Cup", UWCL "UEFA Champions League. Women", FACUP "England. FA Cup", COPADELREY "Spain. Copa del Rey", COPPAITALIA "Italy. Coppa Italia", CDF "France. Coupe de France", LEAGUESCUP "Leagues Cup", SUPERCOPA "Spain. Supercopa", NCAAB "NCAA" (basketball), tennis "ATP. Miami", "ATP. French Open", "ATP. Wimbledon", "ATP. Australian Open", "ATP. US Open", the same with "WTA. ", and "Challenger. Zadar/Murcia/Morelos/Asuncion". Exact matching keeps doubles, qualifying and outright championships out.
- Game prefix (excluding names with ". Winner", "Special", "Statistic", "Results", "Alternative"): CS2 "CS 2. ", DOTA2 "Dota 2. ", LOL "League of Legends. ", VALORANT "Valorant. ".

## Sub games

A GetGameZip response only holds full-game markets. Periods and props live in sub games listed in `Value.SG` (`TG` topic, `P` period); Price fetches the main game plus the sub games below (`subgames.go`), keyed on the main game's `SI`:

| Sport | Sub game (`TG`, `P`) | Scope |
| --- | --- | --- |
| football | ("", 1), ("Corners", none), ("Players' stats", none) | 1H, corners, players |
| basketball, NFL | ("", 11) 1H, ("", 1..4) Q1..Q4, ("Players' stats", none) | periods, players |
| tennis | ("", 1) S1, ("Aces", none), ("Double Faults", none) | first set, aces, double faults |
| esports | ("", 1) M1, ("Players' stats", 1) | first map, map 1 players |

## Market mapping (`markets.go`, keyed by sport + scope + group `G`, outcome type `T`)

Group and outcome names come from the feed's own dictionary `https://v2l.traincdn.com/genfiles/cms/betstemplates/betsNames_full_en.js` (`betsModelGroup` = group names, `betsModel` = outcome names), loaded by the 22bet.ng page; football groups were first matched against SportyBet prices and the dictionary confirms them.

| Sport / scope | G | Dictionary name | Canon | T -> selection | Line |
| --- | --- | --- | --- | --- | --- |
| football | 1 | 1x2 | 1X2 | 1/2/3 | |
| football | 8 | Double Chance | DC | 4=1X, 5=12, 6=X2 | |
| football | 17 / 99 | Total / Asian Total | OU | 9/10, 3827/3828 | P |
| football | 19 | Both Teams To Score | BTTS | 180 yes, 181 no | |
| football | 2 / 2854 | Handicap / Asian Handicap | AH | 7/8, 3829/3830 (P is own handicap, away negated) | home handicap, missing P = 0 |
| football | 2, line 0 | | DNB | copy of the level handicap market | |
| football | 136 | Correct Score | CS | 731 with P = home + away/1000; 3786 = Other | |
| football | 100 | To Qualify | QUALIFY | 794 home, 795 away | |
| football 1H | 1 | 1x2 | 1H_1X2 | 1/2/3 | |
| football corners | 17 | Total | CORN_OU | 9/10 | P |
| football players | 119 / 117 / 118 | Who Will Score / First / Last goal | P_ANYTIME / P_FIRST / P_LAST | 5869 yes 5870 no / 833 / 834 | |
| football players | 3357 / 3217 | Total Shots Towards The Goal / On Target | P_SHOTS / P_SOT | 4650 / 4416 over | P |
| football players | 9120 | Player's Total Tackles | P_TACKLES | 9290 over, 9291 under | P |
| football players | 11143 | Player To Provide An Assist | P_AST | 15621 over | 0.5 |
| basketball, NFL | 101 | Team Wins (incl. overtime) | ML | 401 home, 402 away | |
| basketball, NFL | 2 | Handicap | SPREAD | 7/8 | home handicap |
| basketball, NFL | 17 | Total | TOTAL | 9/10 | P |
| basketball, NFL 1H/Qn | 2 / 17 | Handicap / Total | SPREAD / TOTAL with period; level handicap also copied to ML with that period (tie = push, like a period DNB) | | |
| basketball players | 850 / 749 / 751 / 755 / 7039 | Players. Total Points / Total Rebounds / Total Assists / Players. Total Three-Point Field Goals / Player, Total Points + Rebounds + Assists | BK_PTS / BK_REB / BK_AST / BK_3PM / BK_PRA | 1807/1806, 1489/1490, 1491/1492, 1495/1496, 5675/5676 over/under | P |
| NFL players | 8707 / 7525 / 8711 / 8709 / 8713 | Total Passing Yards / Touchdown Passes / Rushing Yards / Receiving Yards / Receptions | NF_PASS_YDS / NF_PASS_TD / NF_RUSH_YDS / NF_REC_YDS / NF_REC | 8397/8398, 6376/6377, 8393/8394, 8395/8396, 8399/8400 | P |
| NFL players | 7407 | Player To Score Touchdown Anytime | NF_ANYTIME_TD | 6229 yes | |
| tennis | 1 | 1x2 | ML | 1 home, 3 away | |
| tennis | 109 | Sets Handicap | SET_HCP | 732/733 | home handicap |
| tennis | 17 | Total (games) | TOTAL_GAMES | 9/10 | P |
| tennis S1 | 1 / 17 | | ML / TOTAL_GAMES period S1 | | |
| tennis aces / double faults | 15 / 62 | Individual Total 1 / 2 | TN_ACES / TN_DF, player = home / away competitor (`O1I`/`O2I`) | 11/12, 13/14 | P |
| esports | 1 | 1x2 | ML | 1/3 | |
| esports | 2438 | Total Maps Handicap | MAP_HCP | 2826/2827 | home handicap |
| esports | 2436 | Total Maps | TOTAL_MAPS | 2824/2825 | P |
| esports M1 | 1 | | ML period M1 | | |
| esports map 1 players | 9562 | Player's Total Frags | ES_KILLS period M1 | 10816/10817 | P |

Player bets carry `PL {N name, I id, T team side 1/2}` (esports players have no `T`, so no team). Blocked (`B`) bets and odds `<= 1` are skipped; unknown groups go to `DropUnknownMarket`. Rules are `code.DefaultRule()`.

## Booking codes (`booking.go`)

All four brands implement `sources.Booker` through the coupon "Save/Share" call of the shared platform. It is public: no login, no cookie, no bet. Found in the betting app bundle (`sys-betting-app-static/.../entry-*.js`, `saveCoupon` / `loadCoupon` / `useCouponShare`) and checked live on 2026-10-26.

| Call | Method and URL | Body |
| --- | --- | --- |
| Save | `POST <bookingBase>/service-api/LiveBet/Open/SaveCoupon` | `{"Events":[{"GameId","Type","Coef","Param","PlayerId","Kind":3,"InstrumentId":0,"Seconds":0,"Price":0,"Expired":0}],"Summ":0,"Vid":0 single or 1 accumulator,"partner":<p>,"Lng":"en","UserId":0,"CheckCf":1,"notWait":true}` |
| Load (verification only) | `POST <bookingBase>/service-api/LiveBet/Open/GetCoupon` | `{"Guid":"<code>","Lng":"en","partner":<p>}` (keys are case sensitive: lowercase `lng` answers in Russian) |

- Save answers `{"Value":"<code>","Id":0,"Success":true,"Error":"","ErrorCode":0}`. The code is five characters (`F2NPX`). A transient `{"Success":false,"Error":"Call failed, please try again later.","ErrorCode":161627}` comes back now and then; the adapter retries it up to 3 times, 500 ms apart.
- Load answers `Value.Events[]` with `GameId`, `MainGameId`, `Type`, `Param`, `Coef`, `PlayerId`, `GroupName`, `MarketName`, `PeriodName`, `Opp1`/`Opp2`, `Block`, and `Value.Vid`. Once a game has started it answers `ErrorCode 159271` "Events in the downloaded bet slip have finished".
- `Kind` 3 is a pre-match line event (1 is live and loads back as finished for a pre-match game).
- Each leg is resolved by fetching the leg's event exactly as Price does (GetGameZip for the main game plus the mapped sub games) and running the same group rules from `markets.go`. The offered bet that maps to the leg's code, period, side or label, line and player gives the native `GameId` (the sub game id for 1H, corners and player markets), `Type`, `Param` and `PlayerId`, and its current `Coef` is returned as the leg price. DNB and period moneylines match the level handicap market they are copied from. Player names match on full name, or on surname plus initial (`C. Wood` = `Chris Wood`).
- Legs whose event is not listed or whose selection is missing or blocked come back `OK=false` with a reason; the rest are booked. If nothing is bookable Book returns an error, wrapped in `sources.ErrBookingBlocked` when the book answered 403.
- `ExpiresAt` is the earliest kickoff among the booked games, because the code stops loading once a game starts.
- Web link: `<webUrl>/?coupon-code=<code>`. The site's home route reads `coupon-code` and loads the coupon into the bet slip (`COUPON_CODE = "coupon-code"`). Defaults: https://1xbet.ng, https://22bet.ng, https://paripesa.ng, https://betwinner.ng; override with `extra.webUrl`.
- App link: none. The bundles carry no custom scheme, and neither `/.well-known/assetlinks.json` nor `apple-app-site-association` is served, so the apps only take the code through their "Load bet slip" field.
- `extra.bookingBaseUrl` (default `baseUrl`) moves the GetGameZip lookups and SaveCoupon to another brand domain, for when the main domain serves a Cloudflare challenge. 1xbet.ng redirects its web pages to `https://1xlite-12822.pro`, which answers the same API with the same game ids and partner 159.

## Known gaps

- 1xbet.ng is intermittently behind a Cloudflare challenge (403 "Just a moment..." from a non-NG IP, seen again on 2026-10-26 for GetChampZip and SaveCoupon, while other calls minutes later went through). Fixtures were captured live from 22bet.ng (partner 151): football 2026-10-23, multi-sport 2026-10-24 (NBA, NFL, ATP Almaty, CS2 PGL Bucharest, LoL Worlds, Dota EPL Masters, DFB Pokal), trimmed to mapped groups. paripesa and betwinner return the same ids and prices.
- No NBA game listed points/rebounds/assists player groups on capture day, so the BK_* rows are taken from the dictionary only and covered by a unit test with synthetic bets.
- The multi-sport Discover test maps ATP_MIAMI to ATP Almaty (`leagueIds` override) because no config tennis championship was running; the name-matched codes above are unverified until their tournaments open.
- No HTFT group was found in the feed for EPL fixtures.
- 3085 "Total Fouls On Player" (fouls suffered) is not P_FOULS (fouls committed) and stays unmapped; no card or goal-or-assist player group appeared in the captured feed.

## Event link (`links.go`)

`EventURL` builds `{webURL}/en/line/{sport}/{champId}-{league-slug}/{gameId}-{home}-{away}` (`extra.webUrl` overrides the brand host: 1xbet.ng, 22bet.ng, paripesa.ng, betwinner.ng). Checked 2026-10-27: 22bet with the right champ id opens the exact match; 1xbet.ng redirects to its mirror with the match even with champ `0`; 22bet with champ `0` renders an empty page, so leagues matched by name only (no champ id) may not open on the other brands. Paripesa and betwinner were not reached from this network.
