package seed

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"mybestbet/internal/names"
)

type namedTeam struct {
	id         string
	normalized string
}

var selectEstablishedNamesakes = `
SELECT t.id::text, t."normalizedName" FROM teams t
WHERE t."sofascoreId" IS NOT NULL
	AND string_to_array(t."normalizedName", ' ') && string_to_array($3, ' ')
	AND EXISTS (
		SELECT 1 FROM leagues home JOIN leagues target ON target.code = $2
		WHERE home.country = target.country AND home.sport = target.sport
			AND (home.code = t."leagueCode" OR EXISTS (SELECT 1 FROM team_leagues m WHERE m."teamId" = t.id AND m."leagueCode" = home.code)))
	AND ` + hasNoOtherIDFrom("t.id", "$4")

func findEstablishedNamesake(ctx context.Context, tx pgx.Tx, t SourceTeam) (string, error) {
	normalized := names.Team(t.Name)
	if normalized == "" {
		return "", nil
	}
	rows, err := tx.Query(ctx, selectEstablishedNamesakes, t.Source, t.LeagueCode, normalized, t.SourceID)
	if err != nil {
		return "", fmt.Errorf("find namesake team: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (namedTeam, error) {
		var c namedTeam
		err := row.Scan(&c.id, &c.normalized)
		return c, err
	})
	if err != nil {
		return "", fmt.Errorf("scan namesake teams: %w", err)
	}
	return uniqueNamesake(normalized, candidates), nil
}

func uniqueNamesake(normalized string, candidates []namedTeam) string {
	var exact, similar []string
	for _, c := range candidates {
		switch {
		case c.normalized == normalized:
			exact = append(exact, c.id)
		case isNameTokenSubset(normalized, c.normalized):
			similar = append(similar, c.id)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0]
	case len(exact) == 0 && len(similar) == 1:
		return similar[0]
	}
	return ""
}

func isNameTokenSubset(a, b string) bool {
	if !names.HaveSameSquadQualifiers(a, b) {
		return false
	}
	shorter, longer := strings.Fields(a), strings.Fields(b)
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) == 0 {
		return false
	}
	for _, token := range shorter {
		if !slices.Contains(longer, token) {
			return false
		}
	}
	return true
}
