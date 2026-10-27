# internal/stats/flashscore

Tennis fixtures and per-match serve statistics (aces, double faults) from Flashscore's public web feeds. Used by `cmd/stats` (`stats.TennisCollector`) and `cmd/seed` (tennis fixtures, ESPN as fallback).

## Endpoints (base `https://www.flashscore.com`)

| Call | Path | Notes |
| --- | --- | --- |
| `DayMatches(offset)` | `/x/feed/f_2_{offset}_3_en_1` | every tennis match of one day, offset -7 .. +7 from today; the schedule is published about one day ahead |
| `TournamentResults(code)` | `/tennis/{category}/{tournament}/results/` | HTML page; the latest edition's results sit inline as `cjs.initialFeeds['results'] = { data: \`...\` }` (main draw and qualifying) |
| `ServeStats(matchID)` | `/x/feed/df_st_1_{matchId}` | `SE÷Match` section, rows `SG÷Aces` / `SG÷Double Faults` with `SH` (home) and `SI` (away) |

Feeds answer with an empty body unless the request carries `x-fsign: SW9D1eZo` (the `feed_sign` constant embedded in every Flashscore page); it is set in `config.yaml` `sources.flashscore.headers`. Plain curl with a browser User-Agent works from this network; no browserd needed.

## Feed format

Records are separated by `~`, fields by `¬`, key and value by `÷`. Section records (`ZA` header, `ZL` path such as `/tennis/atp-singles/wimbledon/`) precede their match records (`AA` id, `AD` start unix seconds, `AB` stage 1/2/3 = scheduled/live/finished, `AC` detail 3 finished, 5 cancelled, 8 retired, 9 walkover, `PX`/`PY` participant ids, `AE`/`AF` short names, `WU`/`WV` slugs). Home is the first-listed player.

## Mapping

- Every tour singles section is produced: `atp-singles/`, `wta-singles/`, `challenger-men-singles/` and `challenger-women-singles/` paths. A path listed in `Tournaments` keeps its configured code (`atp-singles/wimbledon` -> `ATP_WIM`); any other gets a code derived from the path (`atp-singles/basel` -> `ATP_BASEL`, `challenger-men-singles/sioux-falls` -> `ATP_CH_SIOUX_FALLS`, `challenger-women-singles/courmayeur` -> `WTA_CH_COURMAYEUR`), so the configured Miami and Challenger codes match the derived ones. `Match.Tournament` also carries a name and country read from the `ZA` header (`ATP - SINGLES: Basel (Switzerland), hard (indoor)` -> `ATP Basel`, `Switzerland`; the slug when the header has no venue). Main draw and qualifying share one code. Doubles, ITF, team events and exhibitions are dropped.
- Full names: Flashscore lists `Sinner J.`; the given name comes from the slug (`sinner-jannik` or `arthur-fils`, either order) when the slug contains the surname and the first initial matches, otherwise the short name is kept. Both are stored as team aliases.
- Stats are stored only for status finished (`AC` 3); retired and walkover matches are skipped. A stats feed whose total points played is 0 counts as no stats (`ErrNoStats`).

## Lag and coverage

Per-match stats appear minutes after the match ends; the collector reads them every 30 minutes for days -1 and 0, and once a day (and at start) for days -7 .. 0 plus the results page of every configured tournament's latest edition. Tournaments are discovered from those day feeds, so whatever ATP, WTA and Challenger events are being played are covered without a hand-kept list. Before storing, `stats.TennisCollector` inserts a missing tournament into `leagues` (sport tennis, `ON CONFLICT DO NOTHING`, so config-seeded leagues are never overwritten). History before the 7-day window is only reachable for configured codes (their results pages); matches between 2026-09-13 and the first discovery run were never stored.

## Football (live score fallback)

`StartedFootballMatches(day, now)` reads `/x/feed/f_1_{day}_3_en_1` (every football match of the day, same `x-fsign`) and returns live and finished matches: `AE`/`AF` team names, `AG`/`AH` goals, `AB` stage 2 live / 3 over, `AC` detail 12 first half, 13 second half, 38 half time, 6 extra time, 7 penalties, 3 finished, 10 after extra time, 11 after penalties, 4 postponed. `AO` is the time of the last status change (the start of the current half while live), so the minute is `45 * halves before + minutes since AO + 1`, written `45+2` once past the half's 45 minutes. Used only for football events without a FotMob link.

## Known gaps

- `x-fsign` is a site constant; if Flashscore rotates it, feeds return empty bodies and day feeds parse to zero matches (the seed then falls back to ESPN).
- Discovered leagues exist only in the database, not in `config.yaml`, so the API's config-based sport lookup labels a team whose home league is one of them as football, and sport-filtered search skips it. Players already seen at a configured tournament keep that home league.
- Fixtures in `testdata/flashscore` are real responses trimmed to a few records. `feed_f_2_1_3_en_1.txt` is a real scheduled-day feed whose two section headers (`ZA`, `ZL`) were rewritten to Zadar so that a configured tournament is in play off-season.
