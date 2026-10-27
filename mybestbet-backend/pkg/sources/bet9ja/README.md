# bet9ja

Prematch feed at `https://sports.bet9ja.com/desktop/feapi`. Verified live on 2026-10-26: Akamai answers 403 to curl and wget from a non-Nigerian IP but serves Go's `net/http` client (the ingestor's), so fixtures were captured through `sources.Deps.Get`.

## Endpoints

| Call | Path | Notes |
| --- | --- | --- |
| Sport tree | `GET /PalimpsestAjax/GetSports?DISP=0` | `D.PAL{sportId: {S_DESC, SG{categoryId: {SG_DESC, G{groupId: {G_DESC, D}}}}}}`. Fetched once per Discover |
| Discover | `GET /PalimpsestAjax/GetEventsInGroup?GROUPID=<id>&DISP=0` | `D.E[]` events (`ID`, `DS` "Home - Away", `STARTDATE`, `STARTDATEUTC`), `D.RGM` main market group, `D.GMK[]` market groups |
| Discover fallback | `GET /PalimpsestAjax/GetEventsInGroupV2?GROUPID=<id>&DISP=0&GROUPMARKETID=<GMID>&matches=true` | Only events carrying that market group are listed. Used when the main group lists nothing (NFL/NCAA: main group 29 is empty, 546 "Main OT" lists every game); tries up to 4 groups from `GMK` |
| Price | `GET /PalimpsestAjax/GetEvent?EVENTID=<id>` | `D.O` holds every price keyed `marketCode[@specifier]_outcomeCode` |
| Player props | `GET /PalimpsestAjax/GetEventsInGroup?GROUPID=<players group>&DISP=0` | One "event" per player under sport `Players Soccer`, group `G_DESC` = match `DS`, `D` = kickoff. `DS` is `(Team) Last, First` |

Envelope is `{R, D}`; anything but `R == "OK"` is an error. `STARTDATE` and `STARTDATEUTC` are both UTC (the earlier West Africa Time assumption put every Bet9ja event one hour early, so none matched another book's event).

`GetEventsInGroupV2` with `GROUPMARKETID=1` (soccer's main group) returns no events for basketball (main group 12), tennis (15) or American football; that is why only football used to load.

## Competitions (`competitions.go`, override with `leagueIds` = group id)

Fixed group ids for EPL, CHAMP, LALIGA, SERIEA, BUNDES, LIGUE1, ERED, PRIMEIRA, SPL, MLS. Every other code is looked up in the tree by sport + category + group name. A group matches the exact name, or the name followed by one `, <stage>` suffix (`Liga MX, Apertura`, `K-League 1, Relegation Round`, `NCAA, Regular Season`, `ATP Basel, Switzerland Men Singles`), except `, Women` and doubles. Names checked against the 2026-10-26 tree: UEFA cups, `Brasileiro Serie A/B`, `Primera LPF`, `Super League 1` (Greece), `Superligaen`, `J.League` / `J.League 2`, `South Korea / K-League 1`, Euroleague, Eurocup, NBA, NFL, `NCAA` (American football). Bet9ja lists no esports.

## Market mapping (`markets.go`, keys verified live)

| Key | Canon |
| --- | --- |
| `S_1X2`, `S_DC`, `S_GGNG`, `S_DNB`, `S_HTFT`, `S_CSFT` | 1X2, DC, BTTS, DNB, HTFT, CS |
| `S_OU@l`, `S_OUA@l` / `S_OU1T@l` / `S_OU2T@l` | OU full time / 1H / 2H |
| `S_AH@l` (home handicap) | AH |
| `S_DC1T`, `S_DC2T`, `S_GGNG1T`, `S_GGNG2T`, `S_DNB1T`, `S_DNB2T` | DC, BTTS, DNB with period 1H / 2H |
| `S_1X21T` | 1H_1X2 |
| `S_12QUALIFY` | QUALIFY |
| `S_OUCORNERS@l`, `S_C1HTOU@l`, `S_C2HTOU@l` | CORN_OU full time / 1H / 2H (listed close to kickoff) |
| `B_12`, `B_H@l`, `B_OUN@l` | ML, SPREAD (home handicap), TOTAL, all incl. overtime |
| `B_DNB1T`, `B_1HH`, `B_1HOU` / `B_2HDNB`, `B_2HH`, `B_2HOU` | ML, SPREAD, TOTAL 1H / 2H |
| `B_<n>QD`, `B_<n>QH`, `B_<n>QOU` | ML, SPREAD, TOTAL Q1..Q4 |
| `AF_121`, `AF_H@l`, `AF_T@l` | ML, SPREAD, TOTAL |
| `AF_1HD`, `AF_1HH`, `AF_1HT` / `AF_2HDF`, `AF_2HH`, `AF_2HTF` | 1H / 2H |
| `AF_<n>QD`, `AF_<n>QH`, `AF_<n>QT` (Q4: `AF_4QDR`, `AF_4QHR`, `AF_4QTR`) | Q1..Q4 |
| `T_12`, `T_121P`, `T_SH@l`, `T_OUG@l` | ML, ML S1, SET_HCP, TOTAL_GAMES |
| `GS_ANYSCORER_Yes`, `GS_1STSCORER_1STSCORER`, `GS_LASTSCORER_LASTSCORER` (players group) | P_ANYTIME, P_FIRST, P_LAST |

Every other market code goes to `DropUnknownMarket` once per event.

## Known gaps

- Shots, cards and assists player groups (GMID 289/134/287) listed nothing for matches five days out; not fetched.
- `T_GH` (game handicap) has no canon code.
- Bet9ja Nigeria lists no esports; NCAAB, ACB and the configured tennis tournaments had no group on 2026-10-26.

## Booking (`booking.go`)

| Step | Call |
| --- | --- |
| Resolve legs | `GET /PalimpsestAjax/GetEvent?EVENTID=<id>` (same fetch as Price), then the `markets.go` normaliser; the leg's native id is the odds key (`S_OU@2.5_O`) |
| Create code | `POST https://apigw.bet9ja.com/sportsbook/placebet/BookABetV2?source=desktop`, form field `BETSLIP` = JSON `{BETS:[{BSTYPE:0,TAB:0,NUMLINES:1,COMB:1,TYPE:<legs>,STAKE:0,...,ODDMIN,ODDMAX,ODDS:{"<E_ID>$<key>":odds},FIXED:{}}],EVS:{"<E_ID>$<key>":{eventId,market:"S_OU",sign:"O",sid:"S_OU@2.5_O",id}},IMPERSONIZE:0}` (override the URL with `extra.bookingUrl`) |
| Response | `{status:1,error:{code,message},data:[{RIS:<code>,BETLINES:{"<E_ID>$<key>":{eventName,oddValue,...}},DATE,COUPONCODE}]}`; `status` -1 with `error.code` 103 "Uncorrect request" on a bad slip |
| Web link | `https://sports.bet9ja.com/?bookABetCode=<code>` (no app scheme; the app opens the same link) |
| Load code | `GET https://coupon.bet9ja.com/desktop/feapi/CouponAjax/GetBookABetCoupon?couponCode=<code>` -> `{R:"OK",D:{O:{"<E_ID>$<key>":{E_NAME,M_NAME,SGN,V,...}}}}` |

- Bet9ja accepts one selection per event: two legs on the same event make the whole call fail with 103, so later legs on an event already on the slip come back `OK=false`.
- `sportId` is not needed; the server fills it in.
- Player props are not booked (the players group is only known after Discover).
- Live 2026-10-26: `5XFHGWS` (2 legs), `5XFHSN6` (1 leg, fixture `testdata/bet9ja/booking_5XFHSN6.json`), `5XFJ3KT` (2 legs), `5XFJ3ZQ` (3 legs). `apigw.bet9ja.com` answered plain HTTP from this network; `coupon.bet9ja.com` answers an Akamai 403, so the codes were not loaded back from here; the create response echoes every leg with its event name and odds.

## Event link (`links.go`)

`EventURL` builds `https://sports.bet9ja.com/event/{id}`. Unverified (2026-10-27): the site answers an Akamai 403 from this network, so the path is a best guess.
