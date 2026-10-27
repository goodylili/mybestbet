package seed

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/internal/logos"
	"mybestbet/internal/names"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/canon"
)

const (
	seededAliasConfidence   = 1.0
	defaultKickoffTolerance = 15 * time.Minute
)

type Store struct {
	pool             *pgxpool.Pool
	logos            *logos.Store
	kickoffTolerance func(canon.Sport) time.Duration
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, logos: logos.NewStore(pool), kickoffTolerance: func(canon.Sport) time.Duration { return defaultKickoffTolerance }}
}

func (s *Store) UpsertLeagueLogo(ctx context.Context, leagueCode, logoURL string) error {
	return s.logos.UpsertCandidate(ctx, s.pool, logos.KindLeague, leagueCode, logoURL)
}

func (s *Store) WithKickoffTolerance(toleranceFor func(canon.Sport) time.Duration) *Store {
	s.kickoffTolerance = toleranceFor
	return s
}

func (s *Store) UpsertLeague(ctx context.Context, l League) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO leagues (code, name, country, "sofascoreId", sport) VALUES ($1, $2, $3, NULLIF($4, 0), COALESCE(NULLIF($5, ''), 'football'))
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name, country = EXCLUDED.country, "sofascoreId" = EXCLUDED."sofascoreId",
			sport = CASE WHEN $5 = '' THEN leagues.sport ELSE EXCLUDED.sport END
		WHERE (leagues.name, leagues.country, leagues."sofascoreId", leagues.sport) IS DISTINCT FROM
		      (EXCLUDED.name, EXCLUDED.country, EXCLUDED."sofascoreId", CASE WHEN $5 = '' THEN leagues.sport ELSE EXCLUDED.sport END)`,
		l.Code, l.Name, l.Country, l.UniqueTournamentID, string(l.Sport))
	if err != nil {
		return fmt.Errorf("upsert league %s: %w", l.Code, err)
	}
	return nil
}

func (s *Store) UpsertTeam(ctx context.Context, t Team) (string, error) {
	var teamID string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		normalized := names.Team(t.Name)
		if _, err := tx.Exec(ctx, `
			UPDATE teams SET "sofascoreId" = $1
			WHERE id = (
				SELECT t.id FROM teams t
				WHERE t."sofascoreId" IS NULL AND t."normalizedName" = $3 AND `+isLeagueMember+`
				ORDER BY t."createdAt" LIMIT 1)
			  AND NOT EXISTS (SELECT 1 FROM teams WHERE "sofascoreId" = $1)`,
			t.SofascoreID, t.LeagueCode, normalized); err != nil {
			return fmt.Errorf("adopt team: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO teams ("leagueCode", name, "normalizedName", "sofascoreId") VALUES ($1, $2, $3, $4)
			ON CONFLICT ("sofascoreId") DO UPDATE SET name = EXCLUDED.name, "normalizedName" = EXCLUDED."normalizedName"
			RETURNING id`,
			t.LeagueCode, t.Name, normalized, t.SofascoreID).Scan(&teamID); err != nil {
			return fmt.Errorf("insert team: %w", err)
		}
		return s.recordTeamSourceRefs(ctx, tx, SourceTeam{
			Source: sofascore.SourceName, SourceID: strconv.FormatInt(t.SofascoreID, 10), LeagueCode: t.LeagueCode,
			Name: t.Name, Aliases: []string{t.ShortName},
		}, teamID)
	})
	if err != nil {
		return "", fmt.Errorf("upsert team %d: %w", t.SofascoreID, err)
	}
	return teamID, nil
}

func (s *Store) UpsertPlayer(ctx context.Context, p Player) (string, error) {
	var playerID string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		normalized := names.Person(p.Name).Full()
		if _, err := tx.Exec(ctx, `
			UPDATE players SET "sofascoreId" = $1, "updatedAt" = now()
			WHERE "sofascoreId" IS NULL AND "teamId" = $2 AND "normalizedName" = $3
			  AND NOT EXISTS (SELECT 1 FROM players WHERE "sofascoreId" = $1)`,
			p.SofascoreID, nullableID(p.TeamID), normalized); err != nil {
			return fmt.Errorf("adopt player: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO players ("sofascoreId", name, "normalizedName", "teamId", position) VALUES ($1, $2, $3, $4, NULLIF($5, ''))
			ON CONFLICT ("sofascoreId") DO UPDATE SET
				name = EXCLUDED.name,
				"normalizedName" = EXCLUDED."normalizedName",
				"teamId" = `+clubTeamAfterMove+`,
				position = COALESCE(EXCLUDED.position, players.position),
				"updatedAt" = CASE
					WHEN (players.name, players."teamId", players.position) IS DISTINCT FROM
					     (EXCLUDED.name, `+clubTeamAfterMove+`, COALESCE(EXCLUDED.position, players.position))
					THEN now() ELSE players."updatedAt" END
			RETURNING id`,
			p.SofascoreID, p.Name, normalized, nullableID(p.TeamID), p.Position, NationalTeamLeagues()).Scan(&playerID); err != nil {
			return fmt.Errorf("insert player: %w", err)
		}
		return recordPlayerSourceRefs(ctx, tx, sofascore.SourceName, strconv.FormatInt(p.SofascoreID, 10), playerID, p.TeamID)
	})
	if err != nil {
		return "", fmt.Errorf("upsert player %d: %w", p.SofascoreID, err)
	}
	return playerID, nil
}

func (s *Store) UpsertEvent(ctx context.Context, e Event) (string, error) {
	tolerance, err := s.leagueKickoffTolerance(ctx, e.LeagueCode)
	if err != nil {
		return "", err
	}
	var eventID string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE events SET "sofascoreId" = $1
			WHERE "sofascoreId" IS NULL AND "mergedInto" IS NULL AND "homeTeamId" = $2 AND "awayTeamId" = $3
			  AND kickoff BETWEEN $4::timestamptz - $5::interval AND $4::timestamptz + $5::interval
			  AND NOT EXISTS (SELECT 1 FROM events WHERE "sofascoreId" = $1)`,
			e.SofascoreID, e.HomeTeamID, e.AwayTeamID, e.Kickoff, tolerance); err != nil {
			return fmt.Errorf("adopt event: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff, "sofascoreId", status) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT ("sofascoreId") DO UPDATE SET
				"leagueCode" = EXCLUDED."leagueCode", "homeTeamId" = EXCLUDED."homeTeamId", "awayTeamId" = EXCLUDED."awayTeamId",
				kickoff = EXCLUDED.kickoff, status = EXCLUDED.status
			RETURNING id`,
			e.LeagueCode, e.HomeTeamID, e.AwayTeamID, e.Kickoff, e.SofascoreID, string(e.Status)).Scan(&eventID); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		return recordEventSourceRef(ctx, tx, sofascore.SourceName, strconv.FormatInt(e.SofascoreID, 10), eventID)
	})
	if err != nil {
		return "", fmt.Errorf("upsert event %d: %w", e.SofascoreID, err)
	}
	return eventID, nil
}

func (s *Store) leagueKickoffTolerance(ctx context.Context, leagueCode string) (time.Duration, error) {
	var sport string
	if err := s.pool.QueryRow(ctx, `SELECT sport FROM leagues WHERE code = $1`, leagueCode).Scan(&sport); err != nil {
		return 0, fmt.Errorf("league %s sport: %w", leagueCode, err)
	}
	return s.kickoffTolerance(canon.Sport(sport)), nil
}

func nullableID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}
