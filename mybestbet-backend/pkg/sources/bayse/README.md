# bayse

REST at `baseUrl` (`https://relay.bayse.markets`), websocket `wss://socket.bayse.markets/ws/v1/markets` (`extra.wsUrl`). All endpoints used here are public; when `SOURCE_BAYSE_API_KEY` is set it is sent as `X-Public-Key`. The HMAC-SHA256 scheme (`X-Timestamp` + `X-Signature` = base64 HMAC over `{ts}.{METHOD}.{path}.{sha256hex(body)}`) is only required for write endpoints, which this adapter never calls.

## Flow

- Discover (football): `GET /v1/pm/sports/games?league=<league name>&page=..&size=100`, `UNPLAYED` games only. Source event id = game slug `bm-game-YYYYMMDD-<home>-<away>`, whose two codes are the team short codes used in market titles. `GET /v1/pm/sports/leagues` lists every league name the games endpoint knows; it is soccer only.
- Discover (NBA): the games endpoint has no basketball. NBA events are listed under `GET /v1/pm/events?subcategory=NBA&currency=NGN&size=20&page=..` (the `subcategory` filter also matches hashtags). They mirror Kalshi (`rules` cite Kalshi, image urls carry Kalshi tickers): a game is a `COMBINED_MARKETS` event hashtagged `Games` titled `<Away> vs <Home>` (Kalshi NBA order, checked against ESPN: "New Orleans vs Utah" is NO @ UTAH), one Yes/No market per team. Prop events are `GROUPED_MARKETS` titled `<game title>: Points|Rebounds|Assists|Threes` with the same `closingDate`, and are attached to their game by that title prefix + closing date. Source event id = game event slug (`new-orleans-vs-utah-october-24-2026`). `closingDate` is Kalshi's expected expiration, tip-off + 3h (matched ESPN start times for every captured game), so Kickoff = closingDate - 3h; games already started are skipped.
- Price (NBA): one `subcategory=NBA` listing per Price call (only when a non `bm-game-` id is asked for), grouped into game + props as above.
- Price (football): `GET /v1/pm/events?sportGameSlug=<slug>&currency=NGN&size=20&page=..` (the API caps pages at 20) returns every event of the match, including the `PLAYER STATS` category (ratings, Opta props) and team-stat buckets. AMM markets are priced from `outcome1Price`/`outcome2Price` (the buy prices, they carry the vig). CLOB markets use `GET /v1/pm/books?outcomeId[]=..` (USD book, same 0..1 scale) and `NewBookPrice`.
- Prices are shares of a ₦100 payout: Raw = price x 100 (`FormNaira`), Prob = Raw/100.

## Market table

| Bayse | Canon | Notes |
| --- | --- | --- |
| `TEAM_H2H_3WAY` | 1X2 | market title is the short code (`ARS`, `EVE`) or `Draw` |
| `BOTH_TEAMS_TO_SCORE` | BTTS | |
| `GOAL_SPREAD` | AH | winner and margin come from the market `rules` ("If Frosinone wins by more than 1.5 goals in the Atalanta vs Frosinone ..."): home winner -> Line -margin, outcome1 home; away winner -> Line +margin, outcome1 away. Dropped when the rules margin differs from `propLine` or the rules cannot be parsed |
| `TOTAL_GOALS` | OU | `propLine` |
| `TOTAL_CORNERS` | CORN_OU | both the single "10+ corners" and grouped forms |
| `TOTAL_CARDS` | T_CARDS | single over/under line |
| `PLAYER_GOALSCORER` | P_ANYTIME | 0.5 line only |
| `PLAYER_GOALS` / `PLAYER_SHOTS` / `PLAYER_ASSISTS` / `PLAYER_TACKLES` | P_GOALS / P_SHOTS / P_AST / P_TACKLES | `propLine`, else "N+" -> N-0.5 or "O/U x" |
| untyped, title "Sofascore Rating" | P_RATING | "Delap 6.9+ Sofascore Rating vs Ipswich?" -> player Delap, Line 6.9, yes/no; rules `VoidIfNoStart`, `Resolver: sofascore`. The slug is not reliable (`delap-7-4-...` for a 6.9 line) |
| untyped, title "How Many Passes" | T_PASSES | 3 buckets |
| untyped, title "Total Shots" | T_SHOTS | 3 buckets |
| untyped, title "Total Cards" | T_CARDS | 3 buckets |

NBA (hashtag `NBA` plus the stat hashtag):

| Bayse | Canon | Notes |
| --- | --- | --- |
| `Games` | ML | market title is the team name; each team's Yes outcome; rule incl. OT (`ML` default) |
| `Points` / `Rebounds` / `Assists` / `Threes` | BK_PTS / BK_REB / BK_AST / BK_3PM | market title "Brandin Podziemski: 25+" -> player before `:`, over 24.5 = Yes, under = No; incl. OT, `VoidIfNoStart` (Kalshi settles inactive players to a fair price) |

Any other hashtag on an NBA event is dropped.

Bucket labels: "Less than 21" -> `<21`, "21 - 28" -> `21-28`, "29 or more" / "29+" -> `29+`; one selection per bucket priced from its Yes outcome. Opta player props settle on 90 minutes and are void if the player does not start (`VoidIfNoStart`, `Resolver: opta`). Dropped: `TOTAL_GOALS_HOME/AWAY`, `TEAM_CORNERS`, `FIRST_TEAM_TO_SCORE`, `ODD_EVEN_GOALS`, `TEAM_WIN_*`, `PLAYER_SAVES/CROSSES/DRIBBLE_ATTEMPTS/KEY_PASSES/PASSING_ATTEMPTS/CLEARANCES`.

Bayse sometimes lists the same market twice in one game (a `SINGLE_MARKET` "10+ corners" next to the `GROUPED_MARKETS` "Total Corners" line). Events are built most-liquid first and the first market per (code, line, player, period, team) wins.

CLOB prices are `Suspended` unless both sides of the book are quoted, the best ask has size and the spread is at most 0.10 of a share.

## Leagues

| Canon | Bayse |
| --- | --- |
| EPL, FACUP, EFLCUP | England - Premier League / FA Cup / EFL Cup |
| LALIGA, COPADELREY, SUPERCOPA | Spain - La Liga / Copa del Rey / Supercopa |
| SERIEA, COPPAITALIA | Italy - Serie A / Coppa Italia |
| BUNDES, DFBPOKAL | Germany - Bundesliga / DFB Pokal |
| LIGUE1, CDF | France - Ligue 1 / Coupe de France |
| UCL, UEL, UECL | UEFA - Champions League / Europa League / Europa Conference League |
| SPL | Saudi Arabia - Saudi League |
| MLS, LEAGUESCUP | USA - Major League Soccer, North America - Leagues Cup |
| WC, FRIENDLY | FIFA - World Cup, International - Friendlies |
| NBA | events `subcategory=NBA` |

`leagueIds` in config overrides either column (an NBA override replaces the subcategory). Bayse also lists AFC Champions League, Nations League, Kings Cup, US Open Cup, CONCACAF Champions Cup, Club World Cup, WC/Euro qualifiers, Euros, Copa America and Community Shield, which have no canonical code and are not mapped.

## Stream

One `{"type":"subscribe","channel":"prices","eventId":..}` per event (paced under the 10 msg/s limit) plus `orderbook` subscriptions (10 market ids each) for CLOB markets. Frames may hold several newline-separated JSON objects. `price_update` carries the event snapshot with `prices` keyed by upper-case outcome label; `orderbook_update` carries one outcome book. Sessions resync from REST and reconnect every 5 minutes.

## Known gaps

- `GOAL_SPREAD` market titles are unreliable: during the 2026-10-23 smoke run the same market id flipped between "ATA by 2+ Goals" and "FRO by 2+ Goals" between polls (and `propTeam` named the home team on every line), while `rules` and the price stayed consistent. Titles are therefore ignored for the handicap side.

- Every live soccer event was AMM at capture; the BTTS event of `bm-game-20261024-ars-eve` is flipped to `CLOB` in the fixture with a synthesized `books.json` to cover that path. Websocket frames are synthesized from the documented shapes (a live session produced no price updates in 30s).
- `bm-game-20261012-cov-new` holds real legacy player-prop events whose markets were closed; they are reopened with synthesized prices to cover P_GOALS/P_AST.
- Bayse exposes no player ids; `SourcePlayer.SourceID` is the display name (sometimes a surname only).
- T_PASSES is one team's passes ("How Many Passes Will Arsenal Get"), but `MarketKey` has no team field, so it cannot be told apart from the other team's or a match total.
- Team-stat events without a `sportGameSlug` (seen on Saudi games) cannot be linked and are never fetched.
- Championship, Eredivisie, Primeira Liga and every other config league not in the table above are not offered by Bayse. No NFL, tennis, esports, WNBA, NCAA or EuroLeague markets exist (checked every SPORTS event page and `subcategory` for each on 2026-10-24); the only non-football sport is NBA.
- No QUALIFY market: cup ties (DFB Pokal, Copa del Rey, EFL Cup) carry the same 90-minute `TEAM_H2H_3WAY` / BTTS / totals / spread set as league games.
- NBA: only moneyline and the four prop ladders exist (no spread, total or quarter markets). Team names are Kalshi city names ("Los Angeles L", "Los Angeles C", "Golden State"), so the resolver needs those aliases. Player team is not exposed. NBA props are mostly untraded at capture and so priced `Suspended`.
- NBA fixture `events_subcategory_NBA_p1.json` is a real capture trimmed to 4 games and 5 prop events (6 markets each); the `Rebounds` event is synthesized from the Points event (no rebounds ladder was open at capture).

## Event link (`links.go`)

`EventURL` builds `https://bayse.markets/market/{slug}` for NBA games (checked 2026-10-27: title "Detroit vs New York"). Football games (`bm-game-*` ids) fall back to `https://bayse.markets/sports/football`, because their match pages redirect anonymous visitors to sign-in.
