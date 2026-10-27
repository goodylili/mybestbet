# espn

ESPN site API client (`https://site.api.espn.com/apis/site/v2/sports`, no key). Leagues: `Leagues` (NBA, WNBA, NBL, NCAAB group 50, NFL, NCAAF group 80) plus `TennisTours` (atp, wta).

- `Games(league, day)`, `Teams(league)`, `Roster(league, teamID)`, `TennisMatches(tour, day)`, `Boxscore(league, eventID)`.
- `scoreboard?dates=` only accepts a single day (ranges answer 400). Times come as `2026-10-24T22:00Z`.
- Roster: basketball lists athletes flat; football groups them (`{position: "offense", items: [...]}`).
- Boxscore stat keys mapped to `statkey`: points, rebounds, assists, 3PM (made part of `made-attempted`), passing/rushing/receiving yards and TDs, receptions. Football players get every gridiron key, zero when missing from a category. DNP rows are skipped.
- Tennis: combined events (Miami) list both draws on the ATP scoreboard; only singles (`competitor.type = athlete`) are kept; competitor id = athlete id; home = `homeAway: home` (first listed). No player statistics are published.

Fixtures in `testdata/espn` are real responses (2026-10-23/25, Miami 2026-03-25) trimmed of links, logos and odds.
