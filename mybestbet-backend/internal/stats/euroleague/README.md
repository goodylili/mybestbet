# euroleague

Client for the public EuroLeague API (`https://api-live.euroleague.net`): EUROLEAGUE = competition `E`, EUROCUP = `U`. Season code is competition + start year (`E2026` for 2026-27; seasons start in July).

- `Games` (`/v2/competitions/{c}/seasons/{s}/games`, whole season, `utcDate`), `Players` (`/people?personType=J`, active players with club), `GameStats(season, gameCode)` (`/games/{code}/stats`, `timePlayed` in seconds, `startFive`).
- `GameState(season, gameCode)` for live scores: `https://live.euroleague.net/api/Header?gamecode=&seasoncode=` (config `extra.liveBaseUrl`) answers an empty 200 before tip-off, otherwise `Live`, `ScoreA` (home) / `ScoreB` as strings, `Quarter`, `RemainingPartialTime`; it is CDN-cached for 60 s. When the header is not live, `/v2/.../games/{code}` (`played`, `local.score`, `road.score`) decides whether the game is finished.
- Names arrive as `SURNAME, GIVEN`; `DisplayName` turns them into `Given Surname`.

Fixtures in `testdata/euroleague` are real (2026-10-24), trimmed to 4 games and 3 players per club.
