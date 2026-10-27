# internal/stats/acb

Liga ACB (Liga Endesa, league code `ACB`) fixtures and box scores from the JSON API behind acb.com / live.acb.com (`https://api2.acb.com`).

| Call | Endpoint | Used for |
| --- | --- | --- |
| `CurrentWeeks` | `/api/matchdata/Menu/competition-data?competitionIds=1` | weeks (`id`, `startDate`) of the current edition |
| `WeekMatches` | `/api/matchdata/Menu/matchlist?weekId=` | fixtures, kickoff (`startDateTime`), `matchStatus`, home/away team (`clubId` is stable across seasons, `team.id` is per edition) |
| `WeekOf` | (no call) | the current-edition week whose `startDate` is the latest one at or before a time |
| `Boxscore` | `/api/matchdata/Result/boxscores?matchId=` | per-player totals: `statsByPeriods` quarter `0` is the whole game, 1..4 (and OT) are periods; `playTime` is `MM:SS` |

## External facts

- Every API call needs an `X-APIKEY` header. The key is public: it is compiled into the acb.com Next.js bundle (`"X-APIKEY":"<uuid>"` in one of the `/_next/static/chunks/*.js` files referenced by `https://acb.com/es/liga`). The client discovers it at runtime from that page (config `extra.keyPageUrl`), caches it per process and rediscovers it once when the API answers 401/403. It is never stored in config.
- An unplayed match answers `boxscores` with `204 No Content`; `matchFinished` flips to true at the final buzzer.
- There is no public roster endpoint (the site renders squads server-side); the seed builds rosters from each club's latest finished box score, so a player appears once he has been in a match-day squad.
- Player display names use the `nickname` field ("Ricky Rubio", "Boogie Ellis"), the name the league and books print.
- `matchlist` also carries the live state used for live scores: `homeScore`/`awayScore` (null before tip-off), `currentQuarter` (5+ is overtime) and `isOnHalfTime`; there is no game clock.
- Statuses: `NOT_STARTED`, `STARTED`, `FINALIZED`, `POSTPONED`, `SUSPENDED`, `NOT_SCHEDULED`.

Fixtures in `testdata/acb` are real responses (2026-10-24) trimmed to the fields read; the key in `chunk_key.js` is replaced with a test uuid.
