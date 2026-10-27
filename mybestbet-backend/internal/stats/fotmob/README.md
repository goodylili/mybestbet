# internal/stats/fotmob

FotMob gap filler (`https://www.fotmob.com/api/data`): `/leagues?id={leagueId}` for the fixture list (matched to our event by kickoff within 15 min plus either team name), `/matchDetails?matchId={id}` for the lineup (`lineupType` `standard` = confirmed, `predicted` otherwise) and `content.playerStats[*].stats[key=top_stats]` (minutes_played, goals, assists, total_shots, ShotsOnTarget). Ratings are FotMob's own and are never stored. League ids are in `DefaultLeagueIDs`, overridable through `sources.fotmob.leagueIds`.

Fixtures: `testdata/fotmob` (same Tottenham v Coventry match, id 5795484; predicted lineup 5795491).

Logos: FotMob responses reference crests only as `https://images.fotmob.com/image_resources/logo/teamlogo/{teamId}.png` (e.g. `table[].teamForm[].imageUrl`), and league logos follow `.../leaguelogo/{leagueId}.png`; `TeamLogoURL` / `LeagueLogoURL` build those from the ids the fixture list already carries.
