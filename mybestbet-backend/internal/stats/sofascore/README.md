# internal/stats/sofascore

Typed client for the Sofascore JSON API (`https://www.sofascore.com/api/v1`). Used by `cmd/seed` (leagues, teams, squads, upcoming fixtures) and `cmd/stats` (lineups, player match stats, cards). See `../README.md` for endpoint facts and the browser fallback.

| Method | Endpoint |
| --- | --- |
| CurrentSeason | `/unique-tournament/{id}/seasons` (first entry) |
| SeasonTeams | `/unique-tournament/{id}/season/{seasonId}/teams` |
| TeamPlayers | `/team/{id}/players` |
| UpcomingEvents | `/unique-tournament/{id}/season/{seasonId}/events/next/{page}` until past the horizon |
| Event | `/event/{id}` |
| Lineups | `/event/{id}/lineups` |
| Incidents | `/event/{id}/incidents` |

Fixtures: `testdata/sofascore` (Premier League 26/27, Tottenham 3-1 Coventry, event 16363890; predicted lineup for event 16363897).
