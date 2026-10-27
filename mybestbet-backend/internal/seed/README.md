# internal/seed

One-shot, idempotent entity seeding (`cmd/seed`). Football is seeded here from Sofascore with a FotMob fallback; the other sports plug in through `RegisterSportSeeder` (see below).

## Entity model (migration 0017)

- `teams."leagueCode"` is the team's home competition and never moves once set. Every competition a team plays in is a row in `team_leagues ("teamId", "leagueCode")`; the resolver matches a source team against all members of the event's league (home league rows count even without a `team_leagues` row).
- `team_sources (source, "sourceId", "teamId")` and `event_sources (source, "sourceId", "eventId")` hold stable external ids (Sofascore, FotMob, ESPN, esports APIs). Players keep using `player_aliases (source, "sourceId", "playerId")` for the same purpose.
- `team_aliases (source, "sourceName")` are display names; they feed the resolver's fuzzy pool for the team's leagues, so seed every name a book might use (full name, short name, school/location, abbreviation).
- Tennis: every competitor is a team row plus exactly one player row in that team (same name). Doubles pairs are not modelled.
- Esports: a player row's `name` is the in-game handle (`s1mple`, `donk`); books price props by handle and the resolver matches handles exactly.
- National teams are team rows whose home league is the first international competition seeded (WC), with `team_leagues` rows for FRIENDLY etc. Club players called up keep their club as `players."teamId"` and join the national squad through `player_teams`; squad lookups (resolver, `HasPlayers`) read both.
- `cmd/seed` takes `SEED_SPORTS=football,tennis` to seed only some sports.

## Store API (stable, for agent G and anyone else seeding)

```go
type League struct { Code, Name, Country string; Sport canon.Sport; UniqueTournamentID int64 }
type SourceTeam struct { Source, SourceID, LeagueCode, Name string; Aliases []string }
type SourcePlayer struct { Source, SourceID, TeamID, Name, Position string }
type SourceEvent struct { Source, SourceID, LeagueCode, HomeTeamID, AwayTeamID string; Kickoff time.Time; Status EventStatus }

func (s *Store) UpsertLeague(ctx context.Context, l League) error
func (s *Store) UpsertSourceTeam(ctx context.Context, t SourceTeam) (teamID string, err error)
func (s *Store) UpsertSourcePlayer(ctx context.Context, p SourcePlayer) (playerID string, err error)
func (s *Store) UpsertCompetitor(ctx context.Context, t SourceTeam) (teamID, playerID string, err error)
func (s *Store) UpsertSourceEvent(ctx context.Context, e SourceEvent) (eventID string, err error)
```

- `UpsertLeague` writes `leagues.sport`; `UniqueTournamentID` 0 stores NULL `sofascoreId`.
- `UpsertSourceTeam` finds the team by `team_sources(Source, SourceID)`, else adopts a team that is a member of `LeagueCode` with the same normalized name (`names.Team`), else inserts one with home league `LeagueCode`. It then records the source id, the `team_leagues` membership and an alias for `Name` and each of `Aliases`. Safe to replay.
- `UpsertSourcePlayer` finds the player by `player_aliases(Source, SourceID)`, else adopts a player of `TeamID` with the same normalized name (`names.Person(...).Full()`), else inserts. A transfer moves `teamId`.
- `UpsertCompetitor` is the tennis helper: `UpsertSourceTeam` + `UpsertSourcePlayer` with the same source id and name.
- `UpsertSourceEvent` finds the event by `event_sources(Source, SourceID)`, else adopts an event between the same two teams (either orientation) within the league's kickoff tolerance (15 min, tennis 12 h), else inserts. Kickoff and status are updated on replay.

## Plugging in another sport

```go
type SeederDeps struct {
    Store      *Store
    Config     config.Config
    Log        *slog.Logger
    Now        func() time.Time
    SourceDeps func(source string) (sources.Deps, error)
}
type LeagueSeeder interface { SeedLeague(ctx context.Context, league League) error }
func RegisterSportSeeder(sport canon.Sport, build func(SeederDeps) (LeagueSeeder, error))
```

Call `RegisterSportSeeder` from an `init()` in a file of package `seed`. `cmd/seed` upserts every config league, then hands each non-football league to the seeder registered for its sport (football always runs here). `SourceDeps(name)` builds rate-limited `sources.Deps` from `sources.<name>` in config.yaml (browserd fallback included).

## Football

For every football league: upsert `leagues`, then try Sofascore (season teams + squads, next 7 days of fixtures), and fall back to FotMob when Sofascore fails or the league has no Sofascore id. Domestic leagues are seeded before cups, continental and national-team competitions so that cup teams are found through their FotMob/Sofascore id instead of being created twice.

- Sofascore unique tournament ids: `DefaultUniqueTournaments`, overridable via `sources.sofascore.leagueIds`. The ids for the wave-3 competitions are from Sofascore's public tournament URLs; Sofascore answers 403 from this network, so they were not checked against the live API.
- FotMob league ids: `DefaultFotMobLeagues` (all 45 football codes, each verified live on 2026-10-24 against `/api/data/leagues?id=`), overridable via `sources.fotmob.leagueIds`.
- FotMob placeholders for undecided playoff slots ("3rd ranked", "Winner QF1") are skipped.
- FotMob fallback team lookup: `team_sources(fotmob, id)` → `team_aliases(fotmob, name)` → same normalized name among the league's members (domestic leagues only) → create. For cups, a club is only reused through its id or exact FotMob alias, because names repeat across countries (FotMob lists Portugal's and Uruguay's "Nacional").
- Squads are fetched only for teams with no players yet.

`Store.UpsertPlayer` is reused by `cmd/stats` for players that first appear in a lineup.

## Other sports (`othersports*.go`)

`NewOtherSportsSeeder` is registered for basketball, tennis, american-football and esports. Each run seeds fixtures with kickoff in now -3 h .. +7 days.

| Leagues | Source | Ids |
| --- | --- | --- |
| NBA, WNBA, NBL, NCAAB (group 50), NFL, NCAAF (group 80) | ESPN site API scoreboard (one request per day), `/teams` (pro leagues: every team), `/teams/{id}/roster` for teams with a fixture | team `espn` `<league>:<id>`, event `espn` `<league>:<id>`, player `espn` `<league>:<athleteId>` |
| ATP_*/WTA_* (incl. the four Challengers) | Flashscore tennis day feeds (days -1 .. +7), sections mapped by tournament path; ESPN below when Flashscore fails or has no match for the league | competitor (team + player, aliases full and short name) `flashscore` `<participantId>`, event `flashscore` `<matchId>` |
| ATP_*/WTA_* fallback | ESPN `tennis/atp` + `tennis/wta` scoreboards, singles only; tournaments map by name (Miami Open, Roland Garros/French Open, Wimbledon, Australian Open, US Open), men's draw → ATP_, women's → WTA_ | competitor (team + player) `espn` `tennis:<athleteId>`, event `espn` `tennis:<competitionId>` |
| EUROLEAGUE, EUROCUP | api-live.euroleague.net season games, `people?personType=J` | team `euroleague` `E:<club>`/`U:<club>`, event `<season>:<gameCode>`, player person code |
| CS2, DOTA2, VALORANT, LOL | Liquipedia `Liquipedia:Matches` (parse API, upcoming tab, TBD skipped); Dota rosters from OpenDota (`/teams` name match, current members) | team `liquipedia` `<league>:<name>` (+ `opendota` team id), event `liquipedia` `<league>:<Match ID or ts:team1:team2>`, player `opendota` account id |
| CS2, VALORANT, LOL (also) | bo3.gg `matches` (tiers `sources.bo3.extra.tiers`, default `s,a`) from now -30 days to +7 days, rosters (`teams/{slug}?with=players`, coaches skipped) for teams with an unfinished series | team `bo3` team id, event `bo3` `<league>:<slug>`, player `bo3` player id |
| ACB | api2.acb.com (key discovered from the acb.com bundle) `competition-data` weeks up to +7 days, `matchlist?weekId=` per week (whole season so far), rosters from each club's latest finished `boxscores` | team `acb` club id (aliases short name + abbreviation), event `acb` match id, player `acb` license id |

Team aliases: pro ESPN teams get short name + abbreviation, college teams get the school (location). Esports teams get the Liquipedia short name. Liquipedia asks for at most one parse request per 30 s (`maxRps: 0.033`) and a descriptive User-Agent; its upcoming tab lists about 50 matches, often less than 7 days.

Player positions are normalized per sport by `NormalizePosition` before every upsert (`players_position_check`, migration 0020): football `GK DF MF FW` (Sofascore letters, FotMob squad groups), basketball `G F C` (ESPN abbreviations, EuroLeague position names, first part of `F-C`), american football `QB RB WR TE OL DEF K P LS` (ESPN). ACB boxscores, bo3.gg, OpenDota and tennis competitors carry no position or role, so those players stay null. A source position replaces the stored one; a source without one never clears it.
