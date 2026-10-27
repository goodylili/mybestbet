package resolve

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/internal/names"
	"mybestbet/pkg/canon"
)

var errEntityMissing = errors.New("resolve: entity missing")

type store struct {
	pool *pgxpool.Pool
}

func (s *store) loadInto(ctx context.Context, c *Cache, eventsSince time.Time) error {
	loaders := []struct {
		name string
		load func(context.Context, *Cache, time.Time) error
	}{
		{"leagues", s.loadLeagues},
		{"teams", s.loadTeams},
		{"team leagues", s.loadTeamLeagues},
		{"team aliases", s.loadTeamAliases},
		{"events", s.loadEvents},
		{"event links", s.loadEventLinks},
		{"players", s.loadPlayers},
		{"player teams", s.loadPlayerTeams},
		{"player aliases", s.loadPlayerAliases},
	}
	for _, l := range loaders {
		if err := l.load(ctx, c, eventsSince); err != nil {
			return fmt.Errorf("load %s: %w", l.name, err)
		}
	}
	return nil
}

func forEachRow(ctx context.Context, pool *pgxpool.Pool, sql string, args []any, scan func(pgx.Rows) error) error {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *store) loadLeagues(ctx context.Context, c *Cache, _ time.Time) error {
	return forEachRow(ctx, s.pool, `SELECT code, sport FROM leagues`, nil, func(rows pgx.Rows) error {
		var code, sport string
		if err := rows.Scan(&code, &sport); err != nil {
			return err
		}
		c.addLeague(code, canon.Sport(sport))
		return nil
	})
}

func (s *store) loadTeamLeagues(ctx context.Context, c *Cache, _ time.Time) error {
	return s.forEachTeamLeague(ctx, `SELECT "teamId"::text, "leagueCode" FROM team_leagues`, nil, c)
}

func (s *store) loadLeaguesOfTeam(ctx context.Context, c *Cache, teamID string) error {
	return s.forEachTeamLeague(ctx, `SELECT "teamId"::text, "leagueCode" FROM team_leagues WHERE "teamId" = $1`, []any{teamID}, c)
}

func (s *store) forEachTeamLeague(ctx context.Context, sql string, args []any, c *Cache) error {
	return forEachRow(ctx, s.pool, sql, args, func(rows pgx.Rows) error {
		var teamID, league string
		if err := rows.Scan(&teamID, &league); err != nil {
			return err
		}
		c.joinLeague(teamID, league)
		return nil
	})
}

func (s *store) loadPlayerTeams(ctx context.Context, c *Cache, _ time.Time) error {
	return s.forEachSquadMember(ctx, `SELECT "teamId"::text, "playerId"::text FROM player_teams`, nil, c)
}

func (s *store) loadTeamsOfPlayer(ctx context.Context, c *Cache, playerID string) error {
	return s.forEachSquadMember(ctx, `SELECT "teamId"::text, "playerId"::text FROM player_teams WHERE "playerId" = $1`, []any{playerID}, c)
}

func (s *store) forEachSquadMember(ctx context.Context, sql string, args []any, c *Cache) error {
	return forEachRow(ctx, s.pool, sql, args, func(rows pgx.Rows) error {
		var teamID, playerID string
		if err := rows.Scan(&teamID, &playerID); err != nil {
			return err
		}
		c.addSquadMember(teamID, playerID)
		return nil
	})
}

func (s *store) loadTeams(ctx context.Context, c *Cache, _ time.Time) error {
	return forEachRow(ctx, s.pool, `SELECT id::text, "leagueCode", name FROM teams`, nil, func(rows pgx.Rows) error {
		var t Team
		if err := rows.Scan(&t.ID, &t.League, &t.Name); err != nil {
			return err
		}
		c.addTeam(t)
		return nil
	})
}

func (s *store) loadTeamAliases(ctx context.Context, c *Cache, _ time.Time) error {
	return forEachRow(ctx, s.pool, `SELECT source, "sourceName", "teamId"::text FROM team_aliases`, nil, func(rows pgx.Rows) error {
		var source, name, teamID string
		if err := rows.Scan(&source, &name, &teamID); err != nil {
			return err
		}
		c.addTeamAlias(source, name, teamID)
		return nil
	})
}

const eventColumns = `id::text, "leagueCode", "homeTeamId"::text, "awayTeamId"::text, kickoff`

func scanEvent(row pgx.Row) (Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.League, &e.HomeTeamID, &e.AwayTeamID, &e.Kickoff)
	return e, err
}

func (s *store) loadEvents(ctx context.Context, c *Cache, since time.Time) error {
	return forEachRow(ctx, s.pool, `SELECT `+eventColumns+` FROM events WHERE kickoff >= $1 AND "mergedInto" IS NULL`, []any{since}, func(rows pgx.Rows) error {
		e, err := scanEvent(rows)
		if err != nil {
			return err
		}
		c.addEvent(e)
		return nil
	})
}

func (s *store) loadEventLinks(ctx context.Context, c *Cache, since time.Time) error {
	return forEachRow(ctx, s.pool, `SELECT l.source, l."sourceEventId", l."eventId"::text, l.flipped
		FROM event_links l JOIN events e ON e.id = l."eventId" WHERE e.kickoff >= $1 AND e."mergedInto" IS NULL`, []any{since}, func(rows pgx.Rows) error {
		var (
			source, sourceEventID string
			link                  EventLink
		)
		if err := rows.Scan(&source, &sourceEventID, &link.EventID, &link.Flipped); err != nil {
			return err
		}
		c.linkEvent(source, sourceEventID, link)
		return nil
	})
}

const playerColumns = `id::text, name, COALESCE("teamId"::text, '')`

func (s *store) loadPlayers(ctx context.Context, c *Cache, _ time.Time) error {
	return forEachRow(ctx, s.pool, `SELECT `+playerColumns+` FROM players`, nil, func(rows pgx.Rows) error {
		var p Player
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID); err != nil {
			return err
		}
		c.addPlayer(p)
		return nil
	})
}

func (s *store) loadPlayerAliases(ctx context.Context, c *Cache, _ time.Time) error {
	var aliases []storedPlayerAlias
	err := forEachRow(ctx, s.pool, `SELECT source, "sourceId", "playerId"::text, confidence FROM player_aliases`, nil, func(rows pgx.Rows) error {
		var a storedPlayerAlias
		if err := rows.Scan(&a.source, &a.sourceID, &a.playerID, &a.confidence); err != nil {
			return err
		}
		aliases = append(aliases, a)
		return nil
	})
	if err != nil {
		return err
	}
	for _, a := range uncontestedPlayerAliases(aliases) {
		c.addPlayerAlias(a.source, a.sourceID, a.playerID)
	}
	return nil
}

func (s *store) teamByID(ctx context.Context, id string) (Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx, `SELECT id::text, "leagueCode", name FROM teams WHERE id = $1`, id).Scan(&t.ID, &t.League, &t.Name)
	return t, notFoundAs(err, "team", id)
}

func (s *store) eventByID(ctx context.Context, id string) (Event, error) {
	e, err := scanEvent(s.pool.QueryRow(ctx, `SELECT `+eventColumns+` FROM events WHERE id = $1`, id))
	return e, notFoundAs(err, "event", id)
}

func (s *store) playerByID(ctx context.Context, id string) (Player, error) {
	var p Player
	err := s.pool.QueryRow(ctx, `SELECT `+playerColumns+` FROM players WHERE id = $1`, id).Scan(&p.ID, &p.Name, &p.TeamID)
	return p, notFoundAs(err, "player", id)
}

func notFoundAs(err error, kind, id string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s %s", errEntityMissing, kind, id)
	}
	if err != nil {
		return fmt.Errorf("read %s %s: %w", kind, id, err)
	}
	return nil
}

func (s *store) linkTeamAlias(ctx context.Context, source, sourceName, teamID string, confidence float64) (string, error) {
	var stored string
	err := s.pool.QueryRow(ctx, `INSERT INTO team_aliases (source, "sourceName", "teamId", confidence) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source, "sourceName") DO UPDATE SET "teamId" = team_aliases."teamId"
		RETURNING "teamId"::text`, source, sourceName, teamID, confidence).Scan(&stored)
	if err != nil {
		return "", fmt.Errorf("link team alias: %w", err)
	}
	return stored, nil
}

func (s *store) linkPlayerAlias(ctx context.Context, source, sourceID, playerID string, confidence float64) (string, error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO player_aliases (source, "sourceId", "playerId", confidence) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source, "sourceId") DO UPDATE SET "playerId" = EXCLUDED."playerId", confidence = EXCLUDED.confidence
		WHERE player_aliases.confidence < $5`, source, sourceID, playerID, confidence, exactPlayerNameScore)
	if err != nil {
		return "", fmt.Errorf("link player alias: %w", err)
	}
	var stored string
	err = s.pool.QueryRow(ctx, `SELECT "playerId"::text FROM player_aliases WHERE source = $1 AND "sourceId" = $2`, source, sourceID).Scan(&stored)
	if err != nil {
		return "", fmt.Errorf("read player alias: %w", err)
	}
	return stored, nil
}

func (s *store) linkEvent(ctx context.Context, source, sourceEventID string, link EventLink) (EventLink, error) {
	var stored EventLink
	err := s.pool.QueryRow(ctx, `INSERT INTO event_links (source, "sourceEventId", "eventId", flipped) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source, "sourceEventId") DO UPDATE SET "eventId" = event_links."eventId"
		RETURNING "eventId"::text, flipped`, source, sourceEventID, link.EventID, link.Flipped).Scan(&stored.EventID, &stored.Flipped)
	if err != nil {
		return EventLink{}, fmt.Errorf("link event: %w", err)
	}
	return stored, nil
}

func (s *store) relinkEvent(ctx context.Context, source, sourceEventID string, link EventLink) error {
	_, err := s.pool.Exec(ctx, `UPDATE event_links SET "eventId" = $3, flipped = $4
		WHERE source = $1 AND "sourceEventId" = $2 AND ("eventId", flipped) IS DISTINCT FROM ($3::uuid, $4)`,
		source, sourceEventID, link.EventID, link.Flipped)
	if err != nil {
		return fmt.Errorf("relink event: %w", err)
	}
	return nil
}

func (s *store) upsertEvent(ctx context.Context, e Event) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ($1, $2, $3, $4)
		ON CONFLICT ("homeTeamId", "awayTeamId", kickoff) DO UPDATE SET kickoff = EXCLUDED.kickoff
		RETURNING id::text`, e.League, e.HomeTeamID, e.AwayTeamID, e.Kickoff).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert event: %w", err)
	}
	return id, nil
}

func (s *store) joinLeague(ctx context.Context, teamID, league string) error {
	if _, err := s.pool.Exec(ctx, `INSERT INTO team_leagues ("teamId", "leagueCode") VALUES ($1, $2) ON CONFLICT DO NOTHING`, teamID, league); err != nil {
		return fmt.Errorf("join team %s to league %s: %w", teamID, league, err)
	}
	return nil
}

func (s *store) insertCompetitor(ctx context.Context, source, league, name string) (Team, Player, error) {
	team := Team{League: league, Name: name}
	player := Player{Name: name}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			WITH existing AS (SELECT "teamId" FROM team_aliases WHERE source = $1 AND "sourceName" = $2),
			created AS (
				INSERT INTO teams ("leagueCode", name, "normalizedName") SELECT $3, $2, $4 WHERE NOT EXISTS (SELECT 1 FROM existing)
				RETURNING id)
			SELECT id::text FROM created UNION ALL SELECT "teamId"::text FROM existing`,
			source, name, league, names.Team(name)).Scan(&team.ID)
		if err != nil {
			return fmt.Errorf("insert competitor team: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO team_aliases (source, "sourceName", "teamId", confidence) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			source, name, team.ID, exactPlayerNameScore); err != nil {
			return fmt.Errorf("insert competitor alias: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_leagues ("teamId", "leagueCode") VALUES ($1, $2) ON CONFLICT DO NOTHING`, team.ID, league); err != nil {
			return fmt.Errorf("insert competitor league: %w", err)
		}
		err = tx.QueryRow(ctx, `
			WITH existing AS (SELECT id FROM players WHERE "teamId" = $1 LIMIT 1),
			created AS (
				INSERT INTO players (name, "normalizedName", "teamId") SELECT $2, $3, $1 WHERE NOT EXISTS (SELECT 1 FROM existing)
				RETURNING id)
			SELECT id::text FROM created UNION ALL SELECT id::text FROM existing`,
			team.ID, name, names.Person(name).Full()).Scan(&player.ID)
		if err != nil {
			return fmt.Errorf("insert competitor player: %w", err)
		}
		player.TeamID = team.ID
		_, err = tx.Exec(ctx, `INSERT INTO player_teams ("playerId", "teamId") VALUES ($1, $2) ON CONFLICT DO NOTHING`, player.ID, team.ID)
		return err
	})
	if err != nil {
		return Team{}, Player{}, fmt.Errorf("create competitor %s in %s: %w", name, league, err)
	}
	return team, player, nil
}
