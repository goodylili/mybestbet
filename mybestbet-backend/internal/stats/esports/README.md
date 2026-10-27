# esports

- Liquipedia (`https://liquipedia.net/{wiki}/api.php?action=parse&page=Liquipedia:Matches&format=json&prop=text`): wikis `dota2`, `counterstrike`, `valorant`, `leagueoflegends`. The HTML has an upcoming tab (`data-toggle-area-content="1"`, ~50 matches) and a completed tab. Team full name = `title` of the `.name a` link (minus " (page does not exist)"), short name = its text; TBD opponents have no link and are skipped. Match id from `Match:ID_...` links (CS has none, so `ts:team1:team2`). Liquipedia's terms: at most 1 parse request per 30 s, descriptive User-Agent, gzip (Go's transport sends it).
- OpenDota (`https://api.opendota.com/api`, no key, ~60 req/min): `proMatches` (latest 100 pro games), `matches/{id}` (players with pro `name` and `kills`), `teams`, `teams/{id}/players` (`is_current_team_member`).
- OpenDota `/live` (Steam live league games, top 100 by spectators): `match_id` is a string, `league_id` 0 and team ids 0 for public games, `radiant_score`/`dire_score` are kills, `activate_time` is the game start; there is no series score, so `proMatches` (`radiant_win`, `series_type` 0 bo1, 1 bo3, 2 bo5, 3 bo2) supplies the maps already won. `LiveGames` keeps only active league games between two teams.
- Not wired: HLTV (Cloudflare 403), lolesports persisted gateway (403 without its x-api-key), vlr.gg (HTML only).

Fixtures in `testdata/esports` are real (2026-10-24): Liquipedia pages trimmed to 6 matches with images stripped; one OpenDota series (3 games) with players trimmed to the fields read.
