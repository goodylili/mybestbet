# sporty

One parser for the SportyBet stack, registered twice: `sportybet` (base `https://www.sportybet.com/api/ng`) and `footballcom` (base `https://www.football.com/api/ng`). The only difference between the two is `baseUrl` and `headers` in config.yaml. Football tests for both brands share `testdata/sportybet/`; the multi-sport tests run each brand on its own live capture (`testdata/sportybet/` and `testdata/footballcom/`, goldens `*_multisport`).

## Endpoints

| Call | Path | Notes |
| --- | --- | --- |
| Discover | `POST /factsCenter/pcEvents` body `[{"sportId":"sr:sport:S","marketId":"1","tournamentId":[["sr:tournament:X"],...]}]` | Every listed event of the tournaments, all dates, in one call (no paging). `marketId=1` works for every sport. Replaced `pcUpcomingEvents` on 2026-10-26: its `timeline=168` hid everything past 7 days and its pages overlapped and skipped events (NBA: 19 listed, 13 reachable). |
| Tournament tree | `GET /factsCenter/popularAndSportList?sportId=sr:sport:S&productId=3` | `data.sportList[].categories[].tournaments[]`. Fetched at most once per sport per Discover, only for codes resolved by name. |
| Price | `GET /factsCenter/event?eventId=sr:match:N&productId=3&_t=ms` | `productId=3` is prematch. Full market list under `data.markets`; `data.sport.id` picks the market table. |

Every response is a `{bizCode, message, data}` envelope; anything other than `bizCode=10000` is an error.

Under load from one IP the API soft-throttles by answering `200` with an empty body (seen from Indonesia on 2026-10-26, about one call in three while the ingestor ran). Every call retries an empty body twice (400ms, 800ms) and then fails with `sources.ErrThrottled`, so the runner backs off. A short User-Agent such as `Mozilla/5.0` gets a CloudFront 403 page; keep the full browser UA from config.yaml.

## Competitions (`competitions.go`, override with `leagueIds` = Sportradar tournament id)

Sports: football `sr:sport:1`, basketball `2`, tennis `5`, American football `16`, esports Counter-Strike `109`, League of Legends `110`, Dota `111` (all from `/factsCenter/sportList`), Valorant `194` (not listed on 2026-10-24, so unverified).

- Fixed tournament ids, all seen in the live tree on 2026-10-24: EPL 17, CHAMP 18, FACUP 19, EFLCUP 21, LALIGA 8, COPADELREY 329, SERIEA 23, COPPAITALIA 328, BUNDES 35, DFBPOKAL 217, LIGUE1 34, CDF 335, ERED 37, PRIMEIRA 238, SUPERLIG 52, SPFL 36, JPL 38, MLS 242, LIGAMX 27464, BRA1 325, BRA2 390, ARG1 155, SPL 955, SSL 215, GSL 185, UPL 218, RPL 203, ELITESERIEN 20, DSL 39, J1 196, J2 402, KLEAGUE 410, CSL 649, ISL 1900, ALEAGUE 136, UCL 7, UEL 679, UECL 34480, UWCL 696, LIBERTADORES 384, SUDAMERICANA 480, WC 16, FRIENDLY 851; NBA 132, NCAAB 648, WNBA 486, EUROLEAGUE 138, EUROCUP 141, ACB 264, NBL 1524; NFL 31, NCAAF 27653; slams ATP AO 2567 / RG 2579 / USO 2591 / WIM 2555, WTA AO 2571 / RG 2583 / USO 2595 / WIM 2559.
- By name in the tree (category + all substrings, `Qualif` excluded): ATP_MIAMI / WTA_MIAMI (`ATP`/`WTA`, "Miami" + "Singles"), ATP_CH_ZADAR/MURCIA/MORELOS/ASUNCION (`Challenger`, city + "Singles"), LEAGUESCUP ("Leagues Cup"), SUPERCOPA (`Spain`, "Supercopa"). None of these were running on capture day; the names follow the live pattern (e.g. "ATP Challenger Cali, Colombia Men Singles").
- Whole sport: CS2, LOL, DOTA2, VALORANT (every tournament in the sport's tree). Valorant `194` returns an empty tree on both brands; football.com lists no Counter-Strike events (2026-10-26).

Tennis competitors arrive as `Surname, Given`; Discover turns them into `Given Surname` so they match other books.

## Market mapping (`markets.go`, one table per sport)

Every RawMarket uses `code.DefaultRule()` (football codes regulation only; QUALIFY and every non-football code include extra time/overtime, matching Sporty's "(incl. overtime)" names).

Football:

| Sporty id | Name | Canon | Selections |
| --- | --- | --- | --- |
| 1 | 1X2 | 1X2 | outcome 1/2/3 = home/draw/away |
| 10 | Double Chance | DC | 9=1X, 10=12, 11=X2 |
| 11 | Draw No Bet | DNB | 4=home, 5=away |
| 16 | Asian Handicap | AH, line = `hcp` (home handicap) | 1714=home, 1715=away |
| 18 | Over/Under | OU, line = `total` | 12=over, 13=under |
| 29 | GG/NG | BTTS | 74=yes, 76=no |
| 45 | Correct Score | CS | desc `2:1` -> `2-1`, `Other` |
| 47 | Half Time/Full Time | HTFT | 418..434 -> `1/1` .. `2/2` |
| 60 | 1st Half - 1X2 | 1H_1X2 (period 1H) | 1/2/3 |
| 63 / 85 | 1st / 2nd Half - Double Chance | DC, period 1H / 2H | 9/10/11 |
| 64 / 86 | 1st / 2nd Half - Draw No Bet | DNB, period 1H / 2H | 4/5 |
| 66 / 88 | 1st / 2nd Half - Asian Handicap | AH, period 1H / 2H, line = `hcp` | 1714/1715 |
| 68 / 90 | 1st / 2nd Half - Over/Under | OU, period 1H / 2H, line = `total` | 12/13 |
| 75 / 95 | 1st / 2nd Half - GG/NG | BTTS, period 1H / 2H | 74/76 |
| 81 / 98 | 1st / 2nd Half - Correct Score | CS, period 1H / 2H | desc |
| 166 | Corners - Over/Under | CORN_OU, line = `total` | 12/13 |
| 177 | 1st Half Corners - Over/Under | CORN_OU, period 1H | 12/13 |
| 2 | To Qualify (cup ties, e.g. DFB Pokal) | QUALIFY | 4=home, 5=away |
| 40 | Anytime Goalscorer | P_ANYTIME | one yes selection per player |
| 38 | 1st Goalscorer | P_FIRST | yes |
| 39 | Last Goalscorer | P_LAST | yes |
| 800289 | To Score or Assist | P_GA | yes |
| 800296 | Player To Be Carded | P_CARD | yes (outcome id ends in the team side, not a threshold) |
| 775 | Player goals | P_GOALS | ladder |
| 776 | Player shots | P_SHOTS | ladder |
| 777 | Player shots on goal | P_SOT | ladder |
| 770 | Player assists | P_AST | ladder |
| 780 | Player tackles | P_TACKLES | ladder |
| 800291 | Player Fouls | P_FOULS | ladder |
| 778 | Player passes | P_PASSES | ladder (800292 accurate passes is not mapped) |
| 800297 | Player Saves | P_SAVES | ladder |
| 19 / 20 | `<home>` / `<away>` Over/Under | T_GOALS, team = home / away name, line = `total` | 12/13 |
| 69 / 70 | 1st half - `<team>` Over/Under | T_GOALS, period 1H | 12/13 |
| 91 / 92 | 2nd Half - `<team>` Over/Under | T_GOALS, period 2H | 12/13 |

Basketball and American football (same ids; the 3-way `1`, regulation `18` and `60` are dropped, never read as ML/TOTAL):

| Sporty id | Name | Canon |
| --- | --- | --- |
| 219 | Winner (incl. overtime) | ML, 4/5 |
| 223 | Handicap (incl. overtime) | SPREAD, line = `hcp` (home) |
| 225 | Over/Under (incl. overtime) | TOTAL, line = `total` |
| 64 / 66 / 68 | 1st half DNB / handicap / total | ML / SPREAD / TOTAL, period 1H |
| 86 / 88 / 90 | 2nd half DNB / handicap / total | ML / SPREAD / TOTAL, period 2H |
| 302 / 303 / 236 | quarter DNB / handicap / total (`quarternr`) | ML / SPREAD / TOTAL, period Q1..Q4 |
| 921 / 922 / 923 / 924 | `<player> total points / assists / rebounds / 3-point field goals` (basketball only) | BK_PTS / BK_AST / BK_REB / BK_3PM over/under, player = `player` specifier |
| 768 / 770 / 772 / 774 | Player points / assists / rebounds / 3-pointers ladder (basketball only) | same codes, one `over` per `N+` rung at `N - 0.5`; a rung is dropped when 921-924 already quote that player and line two-sided |
| 914 / 916 / 918 / 919 / 920 | `<player> total passing yards / passing touchdowns / rushing yards / receiving yards / receptions` (NFL) | NF_PASS_YDS / NF_PASS_TD / NF_RUSH_YDS / NF_REC_YDS / NF_REC, player = `player` specifier, name from desc |
| 1235 | Player Touchdowns (ladder) | NF_ANYTIME_TD from the `1+` rung only (yes) |

Tennis: 186 Winner -> ML; 188 Set handicap -> SET_HCP; 189 Total games -> TOTAL_GAMES; 202 / 204 with `setnr=1` -> ML / TOTAL_GAMES period S1. Game handicap (187, 203) has no canon code.

Esports: 186 Winner -> ML (the only prematch market Sporty lists for CS2, LoL and Dota).

Every other id is passed to `deps.DropUnknownMarket`. `1191` "Player to be carded (incl. overtime)" is deliberately unmapped: its prices are a near-flat 2.5-4.1 template per player, while `800296` tracks the other card books (betpawa) player by player (smoke run 2026-10-23: Gronbaek 6.00 vs betpawa 8.30, Capaldo 2.82 vs 3.23; 1191 had 3.90 and 2.55).

Ladder props are sold as one "N+" outcome per threshold, so each becomes its own RawMarket with `Line = N - 0.5` and a single `over` selection.

Players are identified by Sportradar id: scorer outcomes are `sr:player:<id>`, player-prop outcomes are `pre:playerprops:<event>:<player>[:<N>[:<teamSide>]]`; both normalise to `sr:player:<id>`. Names arrive as `Surname, Given (Team)` or `Given Surname (Team)` and are normalised to `Given Surname` with the team split out.

Outcomes with `isActive != 1` or odds `<= 1` and markets with `status != 0` are skipped.

## Booking codes (`booking.go`)

`Book` implements `sources.Booker` for both brands. Found in the site JS (`createCode` in SportyBet's `preMatch/index.*.js`, the `/orders/share` mutation in football.com's Nuxt chunks); no login is needed.

| Step | Call |
| --- | --- |
| Resolve legs | `GET /factsCenter/event?eventId=..&productId=3` per distinct event, then every outcome is run through `rawMarkets` on its own and the first one whose canonical market (code, period, line, player) and selection (side, label) equal the leg wins, so leg mapping is the market normaliser itself |
| Create | `POST /orders/share` with `{"selections":[{"eventId":"sr:match:72221358","marketId":"18","specifier":"total=2.5","outcomeId":"12"}]}` (`specifier` omitted for markets without one) |
| Response | `{"bizCode":10000,"data":{"shareCode":"QP5ABX","shareURL":"http://www.sportybet.com/ng/?shareCode=QP5ABX","deadline":1794322800000,"outcomes":[...],"unavailableOutcomes":[]}}`; football.com answers `{"bizCode":10000,"data":{"shareCode":"SLMB7Z","shareURL":...,"isExpertCode":false}}` with no `deadline` |
| Load (verify) | `GET /orders/share/{code}` returns `data.outcomes[]` with event, `markets[0]` and the booked `outcomes[0]` |

Web links: SportyBet `https://www.sportybet.com/ng/?shareCode={code}&c=ng` (what the share dialog builds), football.com `https://www.football.com/ng/m?shareCode={code}` (the desktop page iframes `/ng/m` and forwards the query). Neither JS bundle has an app scheme for share codes, so `AppURL` is empty. `ExpiresAt` comes from `deadline` (SportyBet: kickoff of the last leg plus about 10 days); football.com gives none. Outcomes listed in `unavailableOutcomes` are reported as not OK. A 403 is returned as `sources.ErrBookingBlocked`.

Live check 2026-10-27 (from Indonesia, no geo block): SportyBet `SNK074` (Chelsea v Man Utd 1X2 home), `QP5ABX` (Chelsea v Man Utd 1X2 home, Bournemouth v Leeds O2.5) and `SMV2FN` (Chelsea DC 1X, Brentford v Forest GG yes, Coventry v Sunderland AH away hcp=0.5, plus an unknown event reported as not offered); football.com `LKMT8L` (Levante v Atletico 1X2 away, Rayo v Celta DC 1X) and `SLMB7Z` (Levante O2.5). All four loaded back through `/orders/share/{code}` with the same selections and odds. Fixtures: `testdata/sportybet/{event_72221358,share_SNK074}.json`, `testdata/footballcom/{event_72478678,share_SLMB7Z}.json`.

## Known gaps

- Football player prop names say "incl. overtime"; for league fixtures there is no overtime so they keep the football default rule (regulation only).
- No Sofascore rating or team stat (`T_*`) markets are mapped.
- No tennis aces/double faults, esports map handicap/total maps or per-map winners were offered prematch on 2026-10-26 (esports lists only 186 Winner), so none are mapped (unknown ids are dropped).
- Football 2nd half 1X2 (83) is not mapped: canon models half results only as `1H_1X2`. 2nd set markets are not mapped (canon has no S2).
- Dropped on purpose because canon has no code: player fouls won (800097), accurate passes (800292, 778), offsides (800280), saves (800297), foot/box/half-specific shots (800231/233/234/261/285), head-to-head player pairs (830xxx `pair=`), first-TD scorer (1234), NFL carries/completions (915/917), team and match stat totals (900xxx, 800054/062-065/262/287).
- NFL player props carry no team (Sporty only gives the player id and name).
- The origin serves two unsynchronised quotes for some player ladders (seen 2026-10-26 on football.com: Umut Günes shots `1.25/2.35/4.60/13` and `1.26/3.00/8.30/44` within seconds of each other, through different CloudFront edges, with no version or timestamp field to tell them apart). `flaps.go` marks a player-prop selection suspended for 15 minutes once it returns to the price it had two polls earlier (A, B, A), so the swings never reach fair prices, moves or signals. Match markets are not guarded.
- Fixtures are real captures: football 2026-10-23, multi-sport 2026-10-24 (NBA, NFL, ATP Basel, CS2, LoL, Dota, DFB Pokal), trimmed to at most three markets per id. The multi-sport Discover test maps ATP_MIAMI to ATP Basel (`leagueIds` override) because no config tennis event was running.

## Event link (`links.go`)

`EventURL` builds `{origin}/ng/sport/{sport}/{Country}/{League_Name}/{Home}_vs_{Away}/{sr:match:id}`; football.com uses `/ng/m/sport/...`. Sport segments: football, basketball, tennis, americanFootball, counterStrike, leagueOfLegends, dota2, valorant. Checked 2026-10-27: sportybet.com answers 200 for the route (the SPA reads the trailing `sr:match` id), but its JS assets timed out from this network so the rendered match page was not seen; non-football sport segments and the football.com path are unverified (football.com was unreachable).
