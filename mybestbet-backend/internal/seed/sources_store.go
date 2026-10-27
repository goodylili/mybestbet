package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"mybestbet/internal/logos"
	"mybestbet/internal/names"
)

const isLeagueMember = `(t."leagueCode" = $2 OR EXISTS (SELECT 1 FROM team_leagues m WHERE m."teamId" = t.id AND m."leagueCode" = $2))`

const clubTeamAfterMove = `CASE
	WHEN players."teamId" IS NOT NULL AND EXISTS (SELECT 1 FROM teams nt WHERE nt.id = EXCLUDED."teamId" AND nt."leagueCode" = ANY($6::text[]))
	THEN players."teamId" ELSE COALESCE(EXCLUDED."teamId", players."teamId") END`

func NationalTeamLeagues() []string {
	var codes []string
	for code, format := range competitionFormats {
		if format == NationalTeams {
			codes = append(codes, code)
		}
	}
	return codes
}

type teamAdoption int

const (
	adoptByIDOrAlias teamAdoption = iota
	adoptByLeagueMemberName
)

type teamCreation bool

const (
	createMissingTeam    teamCreation = true
	linkExistingTeamOnly teamCreation = false
)

func (s *Store) UpsertSourceTeam(ctx context.Context, t SourceTeam) (string, error) {
	return s.upsertSourceTeam(ctx, t, adoptByLeagueMemberName, createMissingTeam)
}

func (s *Store) upsertSourceTeam(ctx context.Context, t SourceTeam, adoption teamAdoption, creation teamCreation) (string, error) {
	var teamID string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		found, err := findSourceTeam(ctx, tx, t, adoption)
		if err != nil {
			return err
		}
		if found == "" && creation == linkExistingTeamOnly {
			return nil
		}
		if found == "" {
			if found, err = insertTeam(ctx, tx, t); err != nil {
				return err
			}
		}
		teamID = found
		return s.recordTeamSourceRefs(ctx, tx, t, teamID)
	})
	if err != nil {
		return "", fmt.Errorf("upsert %s team %s (%s): %w", t.Source, t.Name, t.SourceID, err)
	}
	return teamID, nil
}

func findSourceTeam(ctx context.Context, tx pgx.Tx, t SourceTeam, adoption teamAdoption) (string, error) {
	lookups := []struct {
		sql  string
		args []any
	}{
		{`SELECT "teamId"::text FROM team_sources WHERE source = $1 AND "sourceId" = $2`, []any{t.Source, t.SourceID}},
		{`SELECT a."teamId"::text FROM team_aliases a WHERE a.source = $1 AND a."sourceName" = $2 AND ` + hasNoOtherIDFrom("a.\"teamId\"", "$3"), []any{t.Source, t.Name, t.SourceID}},
	}
	if adoption == adoptByLeagueMemberName {
		lookups = append(lookups, struct {
			sql  string
			args []any
		}{`SELECT t.id::text FROM teams t WHERE t."normalizedName" = $3 AND ` + isLeagueMember + ` AND ` + hasNoOtherIDFrom("t.id", "$4") + `
			ORDER BY t."sofascoreId" NULLS LAST, t."createdAt" LIMIT 1`, []any{t.Source, t.LeagueCode, names.Team(t.Name), t.SourceID}})
	}
	for _, l := range lookups {
		var id string
		err := tx.QueryRow(ctx, l.sql, l.args...).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("find team: %w", err)
		}
		return id, nil
	}
	return findEstablishedNamesake(ctx, tx, t)
}

func hasNoOtherIDFrom(teamIDColumn, sourceIDParam string) string {
	return `NOT EXISTS (SELECT 1 FROM team_sources r WHERE r."teamId" = ` + teamIDColumn + ` AND r.source = $1 AND r."sourceId" <> ` + sourceIDParam + `)`
}

func insertTeam(ctx context.Context, tx pgx.Tx, t SourceTeam) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ($1, $2, $3) RETURNING id::text`,
		t.LeagueCode, t.Name, names.Team(t.Name)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert team: %w", err)
	}
	return id, nil
}

func (s *Store) recordTeamSourceRefs(ctx context.Context, tx pgx.Tx, t SourceTeam, teamID string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO team_sources (source, "sourceId", "teamId") VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		t.Source, t.SourceID, teamID); err != nil {
		return fmt.Errorf("record team source id: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO team_leagues ("teamId", "leagueCode") VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		teamID, t.LeagueCode); err != nil {
		return fmt.Errorf("record team league: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO team_aliases (source, "sourceName", "teamId", confidence)
		SELECT $1, alias, $2, $3 FROM unnest($4::text[]) AS alias WHERE alias <> ''
		ON CONFLICT (source, "sourceName") DO NOTHING`,
		t.Source, teamID, seededAliasConfidence, append([]string{t.Name}, t.Aliases...)); err != nil {
		return fmt.Errorf("record team aliases: %w", err)
	}
	return s.logos.UpsertCandidate(ctx, tx, logos.KindTeam, teamID, t.LogoURL)
}

func (s *Store) UpsertSourcePlayer(ctx context.Context, p SourcePlayer) (string, error) {
	var playerID string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		normalized := names.Person(p.Name).Full()
		err := tx.QueryRow(ctx, `
			SELECT id::text FROM players WHERE id = (SELECT "playerId" FROM player_aliases WHERE source = $1 AND "sourceId" = $2)
			UNION ALL
			(SELECT p.id::text FROM players p
			 WHERE p."normalizedName" = $3 AND (p."teamId" = $4 OR EXISTS (SELECT 1 FROM player_teams pt WHERE pt."playerId" = p.id AND pt."teamId" = $4))
			 ORDER BY p."createdAt" LIMIT 1)
			UNION ALL
			(SELECT p.id::text FROM players p
			 JOIN team_sources club ON club.source = $1 AND club."sourceId" = NULLIF($5, '')
			 WHERE p."normalizedName" = $3 AND (p."teamId" = club."teamId" OR EXISTS (SELECT 1 FROM player_teams pt WHERE pt."playerId" = p.id AND pt."teamId" = club."teamId"))
			 ORDER BY p."createdAt" LIMIT 1)
			LIMIT 1`, p.Source, p.SourceID, normalized, nullableID(p.TeamID), p.ClubSourceID).Scan(&playerID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO players (name, "normalizedName", "teamId", position) VALUES ($1, $2, $3, NULLIF($4, '')) RETURNING id::text`,
				p.Name, normalized, nullableID(p.TeamID), p.Position).Scan(&playerID)
		}
		if err != nil {
			return fmt.Errorf("find or insert player: %w", err)
		}
		if err := moveToClub(ctx, tx, playerID, p); err != nil {
			return err
		}
		if err := recordKnownPosition(ctx, tx, playerID, p.Position); err != nil {
			return err
		}
		return recordPlayerSourceRefs(ctx, tx, p.Source, p.SourceID, playerID, p.TeamID)
	})
	if err != nil {
		return "", fmt.Errorf("upsert %s player %s (%s): %w", p.Source, p.Name, p.SourceID, err)
	}
	return playerID, nil
}

func moveToClub(ctx context.Context, tx pgx.Tx, playerID string, p SourcePlayer) error {
	if p.TeamID == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		WITH previous AS (SELECT "teamId" FROM players WHERE id = $1),
		moved AS (
			UPDATE players SET "teamId" = $2, "updatedAt" = now()
			WHERE id = $1 AND players."teamId" IS DISTINCT FROM $2::uuid
			  AND (players."teamId" IS NULL OR NOT EXISTS (SELECT 1 FROM teams nt WHERE nt.id = $2 AND nt."leagueCode" = ANY($3::text[])))
			RETURNING id)
		DELETE FROM player_teams pt USING previous, moved
		WHERE pt."playerId" = moved.id AND pt."teamId" = previous."teamId"`,
		playerID, p.TeamID, NationalTeamLeagues())
	if err != nil {
		return fmt.Errorf("move player to team: %w", err)
	}
	return nil
}

func recordKnownPosition(ctx context.Context, tx pgx.Tx, playerID, position string) error {
	if position == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE players SET position = $2, "updatedAt" = now() WHERE id = $1 AND position IS DISTINCT FROM $2`, playerID, position); err != nil {
		return fmt.Errorf("record player position: %w", err)
	}
	return nil
}

func recordPlayerSourceRefs(ctx context.Context, tx pgx.Tx, source, sourceID, playerID, teamID string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO player_aliases (source, "sourceId", "playerId", confidence) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source, "sourceId") DO NOTHING`, source, sourceID, playerID, seededAliasConfidence); err != nil {
		return fmt.Errorf("record player alias: %w", err)
	}
	if teamID == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO player_teams ("playerId", "teamId") VALUES ($1, $2) ON CONFLICT DO NOTHING`, playerID, teamID); err != nil {
		return fmt.Errorf("record player team: %w", err)
	}
	return nil
}

func (s *Store) UpsertCompetitor(ctx context.Context, t SourceTeam) (string, string, error) {
	teamID, err := s.UpsertSourceTeam(ctx, t)
	if err != nil {
		return "", "", err
	}
	playerID, err := s.UpsertSourcePlayer(ctx, SourcePlayer{Source: t.Source, SourceID: t.SourceID, TeamID: teamID, Name: t.Name})
	if err != nil {
		return "", "", err
	}
	return teamID, playerID, nil
}

func (s *Store) UpsertSourceEvent(ctx context.Context, e SourceEvent) (string, error) {
	tolerance, err := s.leagueKickoffTolerance(ctx, e.LeagueCode)
	if err != nil {
		return "", err
	}
	if e.Status == "" {
		e.Status = EventScheduled
	}
	var eventID string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT "eventId"::text FROM event_sources WHERE source = $1 AND "sourceId" = $2
			UNION ALL
			(SELECT id::text FROM events
			 WHERE (("homeTeamId", "awayTeamId") = ($3, $4) OR ("homeTeamId", "awayTeamId") = ($4, $3))
			   AND kickoff BETWEEN $5::timestamptz - $6::interval AND $5::timestamptz + $6::interval
			   AND "mergedInto" IS NULL
			   AND NOT EXISTS (SELECT 1 FROM event_sources r WHERE r."eventId" = events.id AND r.source = $1)
			 ORDER BY abs(extract(epoch FROM kickoff - $5::timestamptz)) LIMIT 1)
			LIMIT 1`, e.Source, e.SourceID, e.HomeTeamID, e.AwayTeamID, e.Kickoff, tolerance).Scan(&eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff, status) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT ("homeTeamId", "awayTeamId", kickoff) DO UPDATE SET status = EXCLUDED.status
				RETURNING id::text`, e.LeagueCode, e.HomeTeamID, e.AwayTeamID, e.Kickoff, string(e.Status)).Scan(&eventID)
		}
		if err != nil {
			return fmt.Errorf("find or insert event: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE events SET kickoff = $2, status = $3 WHERE id = $1 AND (kickoff, status) IS DISTINCT FROM ($2, $3)
			AND NOT EXISTS (SELECT 1 FROM events o WHERE o.id <> $1 AND o."homeTeamId" = events."homeTeamId" AND o."awayTeamId" = events."awayTeamId" AND o.kickoff = $2)`,
			eventID, e.Kickoff, string(e.Status)); err != nil {
			return fmt.Errorf("refresh event: %w", err)
		}
		return recordEventSourceRef(ctx, tx, e.Source, e.SourceID, eventID)
	})
	if err != nil {
		return "", fmt.Errorf("upsert %s event %s: %w", e.Source, e.SourceID, err)
	}
	return eventID, nil
}

func recordEventSourceRef(ctx context.Context, tx pgx.Tx, source, sourceID, eventID string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO event_sources (source, "sourceId", "eventId") VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		source, sourceID, eventID); err != nil {
		return fmt.Errorf("record event source id: %w", err)
	}
	return nil
}
