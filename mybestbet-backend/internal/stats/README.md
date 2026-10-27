# internal/stats

Player stats for prop research. Sofascore is primary; FotMob fills gaps (shots, shots on target, lineups) when Sofascore is missing or late; FBref supplies season aggregates and per-90 rates.

## Read API (used by the api service)

```go
r := stats.NewReader(pool)

r.PlayerStats(ctx, playerID string, last int) ([]stats.MatchStat, error)
r.HitRates(ctx, playerID string, code canon.MarketCode, line float64, lastN []int) (stats.HitRates, error)
r.VsOpponent(ctx, playerID, opponentTeamID string, code canon.MarketCode, line float64) (stats.HitRate, []stats.MatchStat, error)
r.MinutesTrend(ctx, playerID string) ([]stats.MinutesPoint, error)
r.LineupStatus(ctx, eventID, playerID string) (stats.LineupStatus, error)
r.RatingDistribution(ctx, playerID string) (stats.RatingDistribution, error)
r.SeasonStats(ctx, playerID string) ([]stats.SeasonStat, error)

r.Histories(ctx, playerIDs []string) (map[string]stats.History, error)
r.LineupStatuses(ctx, eventID string, playerIDs []string) (map[string]stats.LineupStatus, error)
h.HitRates(code, line, lastN) / h.VsOpponent(opponentTeamID, code, line) / h.MinutesTrend() / h.RatingDistribution()
```

The props board loads every player of an event with one `Histories` query (the newest 200 matches per player) and one `LineupStatuses` query, then computes hit rates, vs-opponent, minutes and rating panels in memory. The single-player methods are thin wrappers over the same history.

## Duplicate player merge

`PlayerMerger` runs at startup and every `playerMerge.interval` (config.yaml, default 6h) under a Postgres advisory lock, so only one stats instance merges at a time. It looks for a national-team copy and a club copy of the same person: identical normalised name, plus proof that they are one player. The proof is either a match-stat row of the national copy for the club copy's team, or the national FotMob squad listing that player's club as the club copy's team. Pairs where both copies carry different ids from the same identity source (fotmob, sofascore, espn and the other stat feeds) are never merged, and a copy linked to more than one candidate is skipped as ambiguous.

Each pair is merged in its own transaction. The club copy survives. Aliases, team memberships, match stats, lineups, season stats and prices (`prices_current`, `closing_prices` and `prices_history`, with the market key rewritten) move to it; rows the survivor already has win, and the leftovers are deleted with the duplicate. The duplicate's aliases now point at the survivor, so ingest and seeding resolve to it. A `player_merge` alias update tells resolvers to drop the duplicate from their caches. Running the job again is a no-op.

The live score job also finishes overdue events each round: a `scheduled` or `live` event whose kickoff is older than its sport's maximum duration (football 3 h, basketball 3.5 h, American football 4.5 h, tennis and esports 6 h, otherwise 6 h) and whose live score has not been updated for 30 minutes becomes `finished`. The update is guarded on the current status, so repeating it is a no-op.

Types:

```go
type MatchStat struct {
    PlayerID, EventID, TeamID, OpponentTeamID string
    IsHome bool
    PlayedAt time.Time
    Minutes int
    Started bool
    Goals, Assists, Shots, ShotsOnTarget, Fouls, Tackles, Yellow, Red int
    Rating *float64
    Stats statkey.Values
    ValueSources map[string]string
}

type HitRate struct { Window, Matches, Hits int; Rate float64 }

type HitRates struct {
    PlayerID string
    Code canon.MarketCode
    StatCode canon.MarketCode
    Line float64
    Windows []HitRate
    Home, Away HitRate
}

type MinutesPoint struct { EventID string; PlayedAt time.Time; Minutes int; Started bool }

type LineupStatus string
const LineupUnknown, LineupStarter, LineupBench, LineupOut LineupStatus = "", "starter", "bench", "out"

type RatingDistribution struct { Ratings []float64; Mean, Min, Max float64; Buckets []RatingBucket }
type RatingBucket struct { From, To float64; Count int }

type SeasonStat struct {
    Season, Competition string
    Matches, Starts, Minutes, Goals, Assists, Shots, ShotsOnTarget, Fouls, Tackles, Yellow, Red int
    GoalsPer90, AssistsPer90, ShotsPer90, ShotsOnTargetPer90 float64
    UpdatedAt time.Time
}
```

`ErrInvalidID` is returned when an id is not a uuid. `ErrUnsupportedCode` is returned by `HitRates`/`VsOpponent` for codes with no stat mapping (team stat codes, match markets).

Rules:
- Qualifying matches (`QualifyingMinutes(code)`): football `minutes >= 45`, basketball `minutes >= 10`; american football, tennis and esports count every stored appearance.
- Hit means stat strictly greater than the line (`over`). Yes/No props: P_ANYTIME and P_FIRST/P_LAST map to P_GOALS over 0.5, P_GA to goals+assists over 0.5, P_CARD to yellow+red over 0.5. P_RATING uses rating > line.
- `lastN` defaults to `[5, 10, 20]` when empty. Home/away splits use the largest window.
- PlayerStats is newest first (capped at 100). MinutesTrend covers the last 10 matches (any minutes), oldest first. RatingDistribution covers the last 20 rated matches, newest first; buckets are <6, 6-6.5, 6.5-7, 7-7.5, 7.5-8, >=8.
- Hit-rate windows are the last N qualifying matches. `Home`/`Away` carry `Window` = matches counted.

### Other sports (migration 0018)

Sport stats live in `player_match_stats.stats` (jsonb object, keys from `internal/stats/statkey`). `MatchStat.Stats` carries them; football columns stay as they are. A row only counts toward a code when every key the code reads is present.

| Code | Reads | Notes |
| --- | --- | --- |
| BK_PTS / BK_REB / BK_AST / BK_3PM | `points` / `rebounds` / `assists` / `threes` | |
| BK_PRA | `points + rebounds + assists` | |
| NF_PASS_YDS / NF_RUSH_YDS / NF_REC_YDS / NF_REC / NF_PASS_TD | `passYards` / `rushYards` / `recYards` / `receptions` / `passTDs` | every player in an NFL/NCAAF boxscore gets all gridiron keys (0 when absent from a category) |
| NF_ANYTIME_TD | `rushTDs + recTDs` over 0.5 | yes/no; defensive and return TDs are not in the ESPN boxscore categories we read |
| TN_ACES / TN_DF | `aces` / `doubleFaults` | Flashscore serve stats, configured tournaments only (see `flashscore/README.md`) |
| ES_KILLS | `kills / maps` (kills per map over the series) | Dota 2 (OpenDota), CS2 / Valorant / LoL (bo3.gg) |

Hit means value strictly greater than the line; yes/no codes use line 0.5.

`MatchStat.StatValues()` (the API game log `stats` map) holds the stored `stats` keys, the football columns, and for every prop of the row's sport its `canon` `StatField` set to exactly the value hit rates read, so `kills` there is kills per map (series total = `kills * maps`).

## Pipeline (`cmd/stats`)

One goroutine ticks every minute (`Service.Run`), metrics/healthz on `METRICS_ADDR` (default `:9107`).

| Job | When | Source order | Writes |
| --- | --- | --- | --- |
| Lineups | kickoff -60 min .. +10 min, each event every 5 min | Sofascore (only `confirmed: true`); FotMob (`lineupType: "standard"`) when Sofascore has nothing confirmed 30 min before kickoff | `lineups` starter / bench / out (missing players) |
| Match stats | kickoff +4 h (about 2 h after full time) .. +72 h, retried every 30 min until a Sofascore row exists | Sofascore event + lineups (player statistics) + incidents (cards); FotMob only when Sofascore fails and the event has no rows yet | `player_match_stats`, `events.status` |
| Season stats | daily 04:00 UTC | FBref standard, shooting, misc tables | `player_season_stats` (migration 0008), `player_aliases` source `fbref` |

| Sport stats | kickoff +4 h (esports +8 h) .. +72 h, every 30 min until rows exist | ESPN summary boxscore (NBA, WNBA, NBL, NCAAB, NFL, NCAAF); EuroLeague `games/{code}/stats` (EUROLEAGUE, EUROCUP); Liga ACB `boxscores` (ACB, `internal/stats/acb`); OpenDota `proMatches` + `matches/{id}` (DOTA2); bo3.gg series player stats (CS2, VALORANT, LOL, `internal/stats/bo3`) | `player_match_stats` (`stats` jsonb, `valueSources` = source), `events.status` |
| Tennis stats (`TennisCollector`, own goroutine) | every 30 min (days -1, 0); daily and at start a backfill (days -7 .. 0 + each configured tournament's results page) | Flashscore day feed + `df_st_1_{id}` serve stats | competitors + `events` (`event_sources` `flashscore`), `player_match_stats` (`aces`, `doubleFaults`), skipped when the event already has rows |

Sport stats pick events through `event_sources` (sources `espn`, `euroleague`, `liquipedia`, `bo3`, `acb`) written by `internal/seed` (othersports). Players map through `player_aliases` (`espn` `<league>:<athleteId>`, `euroleague` person code, `opendota` account id, `bo3` player id, `acb` license id); unknown players are created in their side's team. Dota: pro games between the event's two teams (team_sources `opendota`, else name) starting within kickoff -1 h .. +8 h form the series; kills are summed and `maps` counts games. CS2/Valorant/LoL: a `bo3` event (`<league>:<slug>`) reads that series directly; a `liquipedia` event is matched to the bo3 series between the same two teams (team_sources `bo3`, else name) starting within kickoff -1 h .. +8 h. ACB: `acb` event id is the match id; sides come from team_sources `acb` club ids, players who did not play (`00:00`) are skipped.

Backfill: `STATS_BACKFILL=720h go run ./cmd/stats` runs one sport-stats pass over every event with kickoff in now -720 h .. now -4 h that has no rows yet (up to 2000 events) and exits; the seed writes past bo3 series (30 days, tiers from `sources.bo3.extra.tiers`) and the whole ACB season so far, so a backfill gives hit rates real history. The football jobs only look at `leagues.sport = 'football'`.

Merge rules: Sofascore wins every field. FotMob fills fields Sofascore does not supply (minutes, started, goals, assists, shots, shots on target; never rating, fouls, tackles or cards). `valueSources` records the source per field. When Sofascore arrives after a FotMob fallback row, differing values are logged (`stat sources disagree, sofascore wins`) and the row is overwritten. A FotMob write never overwrites a row whose minutes came from Sofascore (guarded in SQL).

Player matching: Sofascore players map by `players.sofascoreId` (unknown players are upserted into the side's team). FotMob and FBref players map by name: exact accent-folded full name, then surname + first initial, disambiguated by team name; FBref only links on an exact full-name match and stores the link as a `fbref` player alias.

## External facts

- Sofascore answers 403 to non-browser clients (TLS fingerprinting; headers alone do not help, `api.sofascore.app` returns empty 200s). A real browser navigating straight to `https://www.sofascore.com/api/v1/...` gets JSON. `internal/stats/fetch` therefore retries any 403 through browserd (`sources.Browser`, profile = source name) and takes the captured response. browserd must allow `sofascore.com` and `fbref.com` (`BROWSERD_ALLOWED_HOSTS`).
- Sofascore unique tournaments (verified 2026-10-23): EPL 17, LaLiga 8, Serie A 23, Bundesliga 35, Ligue 1 34, Eredivisie 37, Primeira 238, Championship 18, Saudi Pro 955, MLS 242. Newest season is first in `/unique-tournament/{id}/seasons`.
- Sofascore lineups carry per-player `statistics` after the match (`minutesPlayed`, `goals`, `goalAssist`, `totalShots`, `onTargetScoringAttempt`, `fouls`, `totalTackle`, `rating`); cards come only from `/event/{id}/incidents` (`incidentClass` yellow, red, yellowRed; a second yellow counts as one red on top of the first yellow). Pre-match lineups are predicted (`confirmed: false`) and ignored.
- FotMob JSON works with plain headers. League ids: EPL 47, LaLiga 87, Serie A 55, Bundesliga 54, Ligue 1 53, Eredivisie 57, Primeira 61, Championship 48, Saudi Pro 536, MLS 130. `matchDetails` omits a top stat when it is zero.
- FBref sits behind a Cloudflare JS challenge (403 to curl) and asks for at most ~10 requests/minute (config `maxRps: 0.15`). Player tables are inside HTML comments. Since 2026 FBref no longer carries Opta advanced data: the defense table's `tackles` column is empty, so tackles come from the misc table's `tackles_won`. Per-90 values are computed from the totals.

- ESPN site API: date ranges in `scoreboard?dates=A-B` answer 400 (2026-10), so the seed asks one day at a time. Tennis scoreboards carry no player statistics and `statsSource` is "none" even for Grand Slams; ESPN does not list Challenger events. EuroLeague, EuroCup and Liga ACB are not on ESPN.
- EuroLeague `api-live.euroleague.net` is open JSON (games, `people?personType=J`, `games/{code}/stats` with `timePlayed` in seconds).
- OpenDota `proMatches` returns only the latest 100 pro games, so a Dota series must be collected within roughly a day.

## Known gaps

- Tennis aces/double faults come only from matches in the 14 configured tournaments (no invented league codes), so a player's history is their Slam, Miami and configured-Challenger matches; other ATP/WTA events are not stored.
- CS2, Valorant, LoL kills come from bo3.gg only for tiers in `sources.bo3.extra.tiers` (default `s,a`); lower-tier Liquipedia series find no bo3 match and stay without rows. HLTV (403, Cloudflare), vlr.gg/gol.gg (HTML), Leaguepedia (anonymous rate limit) and lolesports (gateway key) are not used, see `bo3/README.md`.
- Liga ACB rosters only contain players who have been in a match-day squad this season (no public roster endpoint, see `acb/README.md`).

- Fixtures are real responses trimmed to a few players (Sofascore and FotMob JSON trimmed field-wise; FBref standard and shooting rows verbatim with attributes stripped; the FBref misc fixture keeps only the columns we read).
- Without browserd the Sofascore and FBref jobs fail with 403 and FotMob is the only working source for match stats and lineups.

## Live scores (`LiveScoreCollector`)

Every 30 s the stats process polls events with status scheduled/live and kickoff between now-4h and now+15m, writes `events.score` (canon.EventScore jsonb), `status` (live/finished) and `"liveUpdatedAt"`, and publishes the score as a `canon.BoardUpdate{eventId, score}` on `event:<id>` and as `{eventId, score}` on `board.live`. The UPDATE only fires when score or status differ, the event is still scheduled/live and `"liveUpdatedAt"` is older, so repeated rounds write and publish nothing. `STATS_LIVE_ONCE=1` runs one round and exits.

| Sport | Source | Lookup |
| --- | --- | --- |
| football | FotMob `/matches?date=YYYYMMDD` (UTC days, all leagues in one call; `liveTime.short` such as `23’`, `45+2’`, `HT`) | `event_sources` fotmob id |
| football without a FotMob link | Flashscore football day feeds -1 and 0 (`/x/feed/f_1_{day}_3_en_1`, same `x-fsign` header as tennis; `AB` stage, `AC` detail, `AG`/`AH` goals, `AO` start of the current period) | football events with no `fotmob` `event_sources` row: Flashscore match whose kickoff is within 15 min and whose home and away names match the event's (`internal/names`, squad qualifiers such as U21 must agree, the straight orientation must score higher than the flipped one) |
| basketball, NFL | ESPN `scoreboard?dates=` (US Eastern dates, so the UTC dates of kickoff-4h and -5h; `status.period`, `displayClock`, `type.state` pre/in/post) | `event_sources` espn `league:gameId` |
| EuroLeague, EuroCup | `live.euroleague.net/api/Header?gamecode=&seasoncode=` per game (`Live`, `ScoreA`/`ScoreB`, `Quarter`, `RemainingPartialTime`); when the header is not live, `api-live.euroleague.net` `games/{code}` decides finished (`played`) | `event_sources` euroleague `season:gameCode` |
| Liga ACB | acb.com `matchlist?weekId=` of the week holding the kickoff (`matchStatus` STARTED/FINALIZED, `homeScore`/`awayScore`, `currentQuarter`, `isOnHalfTime`) | `event_sources` acb match id |
| tennis | Flashscore day feeds -1 and 0 (`AG`/`AH` sets, `BA..BJ` set games, `WA`/`WB` game points) | `event_sources` flashscore match id |
| CS2, Valorant, LoL | bo3.gg `/matches` per discipline (`team1_score`/`team2_score` maps won; status `current`, `finished`, `defwin`) | bo3 slug, else Liquipedia events matched by team ids/names within 1 h of kickoff |
| Dota 2 | OpenDota `proMatches` (finished games, `radiant_win`, `series_type` 0 bo1, 1 bo3, 2 bo5, 3 bo2) plus `/live` (league games in progress, `team_id_*`, kills in `radiant_score`/`dire_score`) | Liquipedia/bo3 DOTA2 events: games between the event's two teams (team_sources `opendota`, else name) starting within kickoff -1 h .. +8 h, the same rule as Dota kills |

Basketball periods from EuroLeague and ACB map in one place (`basketballProgress`): quarters 1-4 are `Q1`..`Q4`, later ones `OT`, ACB `isOnHalfTime` is `HT`. Football from Flashscore: `AC` 12/13 first/second half with the minute counted from `AO` (`45+2` past the half), 38 `HT`, 6 extra time, 7 penalties (`ET`, detail `Pen`); finished `AC` 3/10/11 give detail `FT`/`AET`/`Pen`; postponed (`AC` 4) and other details are skipped. Dota scores are maps won with period `M{n}` for the map being played and the live map's kills (home-away) in `detail`; the series is finished when one side reaches the wins its `series_type` needs (bo2 after two maps).

Gaps: bo3.gg exposes no per-map round score (`team*_last_game_score` stays null during live series, `match_maps` is empty), so CS2/Valorant/LoL scores are maps only. OpenDota `/live` drops a game as soon as it ends and `proMatches` lists it a few minutes later, so a deciding map shows the previous map count until then. EuroLeague's header is cached for 60 s at the CDN. The live minute of a FotMob or ESPN game, a live EuroLeague header and a live ACB match were never seen while capturing (2026-10-26 08:30-09:00 UTC); those paths are covered by table tests on the documented fields, and `testdata/euroleague/e_header_70_live_synthetic.json` is the real game 69 header with `Live`, scores, quarter and clock rewritten. `testdata/esports/opendota_live.json` is a real `/live` capture whose third record (a public game) has league and team fields rewritten to the real PARIVISION vs BetBoom series, because no league game was live. Finished and scheduled states use real captures: `testdata/fotmob/matches_20261026.json`, `testdata/espn/nba_scoreboard_20261025.json`, `testdata/euroleague/e_header_69.json` / `e_game_69.json` (and the empty `e_header_89.json` of an unplayed game), `testdata/acb/matchlist_week_2991_finalized.json`, `testdata/esports/opendota_pro_matches_20261025.json`, `testdata/flashscore/feed_f_1_0_3_en_1.txt` and `feed_f_1_-1_3_en_1.txt` (day 0 includes a real live second-half record). `testdata/flashscore/feed_f_2_0_3_en_1.txt` holds real live Ningbo Challenger records under a section header rewritten to Zadar (a configured tournament); `testdata/bo3/cs2_matches_20261026.json` and `match_the-mongolz-vs-mouz-cs2-26-10-2026.json` are real live captures.
