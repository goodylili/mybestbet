# PrizePicks adapter

PrizePicks is DFS pick'em: it posts a line, not a price. Every selection is `canon.NewPrice(line, FormLineOnly, t)`
(Prob 0), side `over`, label = `odds_type` (`standard`, `goblin`, `demon`). Each projection is its own market at its own
line, so a goblin at 1.5 and a standard at 2.5 land on different `MarketKey`s and line up with book prices on the same line.

## Flow

1. Session warm (only when `deps.Browser` is set): browserd opens `extra.sessionUrl` (default `https://app.prizepicks.com/`)
   on profile `prizepicks`, capturing `/projections` and `/leagues`. Its cookies and the forwarded request headers
   (User-Agent, Accept-Language, `X-*`) are cached for `extra.sessionTtl` (default 20m). A 403 drops the cache so the
   next call re-warms. One session serves every board.
2. Boards: one `GET {baseUrl}/projections?league_id=<id>&per_page=1000&single_stat=true` per PrizePicks league board
   needed by the requested canonical leagues (`boards.go`), through `deps.Do` with the session applied. A failing board
   lands in `Snapshot.Errors` and the other boards still return; the call errors only when every board failed.
3. JSON:API join: `data[]` projections -> `relationships.new_player` and `relationships.game` -> `included[]` resources
   of type `new_player` and `game`.

Only projections with `status = pre_game` are priced.

## Boards and leagues

| Board | Default `league_id` | `extra` override key | Canonical league |
| --- | --- | --- | --- |
| SOCCER | 82 | `soccerLeagueId` | from `game.metadata.game_info.competition` |
| TENNIS | 5 | `tennisLeagueId` | from `game.metadata.game_info.competition` (ATP_* / WTA_*) |
| NBA | 7 | `nbaLeagueId` | NBA |
| WNBA | 3 | `wnbaLeagueId` | WNBA |
| CBB | 20 | `cbbLeagueId` | NCAAB |
| NFL | 9 | `nflLeagueId` | NFL |
| CFB | 15 | `cfbLeagueId` | NCAAF |
| CS2 | 265 | `cs2LeagueId` | CS2 |
| Dota2 | 174 | `dota2LeagueId` | DOTA2 |
| VAL | 159 | `valLeagueId` | VALORANT |
| LoL | 121 | `lolLeagueId` | LOL |

Ids 1-176 come from the public league dump in github.com/mada949/PrizePicks-API (`all_leagues.csv`: NBA 7, WNBA 3,
TENNIS 5, NFL 9, CFB 15, CBB 20, SOCCER 82, LoL 121, VAL 159, Dota2 174); NBA 7 and NFL 9 are also confirmed by sample
records of public Apify scrapers. That dump predates CS2 (it lists `CSGO` 124); 265 for CS2 is unverified, which is why
every id is overridable. Competition names for SOCCER and TENNIS are in `markets.go` and can be extended with
`leagueIds: {<CODE>: <competition name>}`. A competition that maps to a league of another sport than its board is dropped.

`SourceEvent.SourceID` is the bare game id on the SOCCER board (unchanged from the football-only adapter) and
`<BOARD>-<gameId>` elsewhere (`NBA-6601`), so `Price` knows which board to fetch without state.

## Stat mapping (`markets.go`, per sport)

| Sport | stat_type -> Code |
| --- | --- |
| Soccer | Goals P_GOALS, Shots P_SHOTS, Shots On Target P_SOT, Assists P_AST, Fouls P_FOULS, Tackles P_TACKLES |
| Basketball | Points BK_PTS, Rebounds BK_REB, Assists BK_AST, 3-PT Made BK_3PM, Pts+Rebs+Asts BK_PRA |
| American football | Pass Yards NF_PASS_YDS, Rush Yards NF_RUSH_YDS, Receiving Yards NF_REC_YDS, Receptions NF_REC, Pass TDs NF_PASS_TD |
| Tennis | Aces TN_ACES, Double Faults TN_DF (the player is the competitor) |
| Esports | MAP 1 Kills ES_KILLS period M1, MAPS 1-2 Kills ES_KILLS period M1-2 (`canon.PeriodMaps1To2`) |

Rules come from `code.DefaultRule()` (football props regulation only, the rest include overtime).
Other stat types (Fantasy Score, Blocked Shots, Rush+Rec TDs, Total Games, MAPS 1-3 Kills, Headshots, Goalie Saves, ...)
are dropped through `DropUnknownMarket`. NF_ANYTIME_TD is a yes/no market and has no line-only equivalent.

## Known gaps

- Fixtures are synthesized: every PrizePicks host answers 403 "unavailable in your location" from this sandbox, and the
  browser warm captured no board call. `testdata/prizepicks/projections*.json` follow the JSON:API shape described by
  public scrapers. The `game.metadata.game_info` competition/teams fields, the tennis competition names, the exact
  `stat_type` spellings for non-soccer boards and the CS2 league id must be verified from a US IP.
- Esports kill lines are kept only for map 1 (M1) and maps 1-2 (M1-2); other map ranges (MAPS 1-3) have no canonical
  period and are dropped rather than treated as the full match.
- Under geo-block the warm succeeds but the board call returns 403 (`sources.ErrForbidden`); a US egress proxy is required.

## Event link (`links.go`)

`EventURL` returns the board `https://app.prizepicks.com/`: projections have no per-game page. Not a match page.
