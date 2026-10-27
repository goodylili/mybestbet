package server

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"mybestbet/internal/logos"
	"mybestbet/internal/names"
	"mybestbet/pkg/canon"
)

const (
	defaultSearchLimit   = 20
	maxSearchLimit       = 50
	minSearchQueryRunes  = 2
	maxSearchQueryRunes  = 64
	prefixMatchRank      = 2
	substringMatchRank   = 1
	eventRankDiscount    = 0.01
	searchQueryParameter = "q"
)

type searchTerms struct {
	team   string
	person string
}

type rankedSearchItem struct {
	SearchItem
	rank           float64
	logoTeamID     string
	awayLogoTeamID string
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	terms, err := searchTermsFrom(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	limit, err := boundedIntQuery(r, "limit", defaultSearchLimit, 1, maxSearchLimit)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	sport, err := sportQuery(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	prefer, err := enumQuery(r, "prefer", sportNames()...)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	items, err := s.searchAll(r.Context(), terms, sport, canon.Sport(prefer), limit)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(items))
}

func searchTermsFrom(r *http.Request) (searchTerms, error) {
	query := strings.TrimSpace(r.URL.Query().Get(searchQueryParameter))
	if runes := utf8.RuneCountInString(query); !utf8.ValidString(query) || runes < minSearchQueryRunes || runes > maxSearchQueryRunes {
		return searchTerms{}, invalidInput(searchQueryParameter, "must be "+strconv.Itoa(minSearchQueryRunes)+"-"+strconv.Itoa(maxSearchQueryRunes)+" characters")
	}
	return searchTerms{team: names.Team(query), person: names.Person(query).Full()}, nil
}

func sortSearchResults(ranked []rankedSearchItem, prefer canon.Sport) {
	slices.SortStableFunc(ranked, func(a, b rankedSearchItem) int {
		return cmp.Or(cmp.Compare(boolRank(b.Sport == prefer), boolRank(a.Sport == prefer)), cmp.Compare(b.rank, a.rank))
	})
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) searchAll(ctx context.Context, terms searchTerms, sport, prefer canon.Sport, limit int) ([]SearchItem, error) {
	ranked := s.matchingLeagues(terms.team, sport)
	if terms.team != "" || terms.person != "" {
		found, err := s.matchingEntities(ctx, terms, s.leagueCodesOf(sport), limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, found...)
	}
	sortSearchResults(ranked, prefer)
	ranked = ranked[:min(limit, len(ranked))]
	if err := s.attachSearchLogos(ctx, ranked); err != nil {
		return nil, err
	}
	items := make([]SearchItem, len(ranked))
	for i, item := range ranked {
		items[i] = item.SearchItem
	}
	return items, nil
}

func (s *Server) matchingLeagues(term string, sport canon.Sport) []rankedSearchItem {
	if term == "" {
		return nil
	}
	var matches []rankedSearchItem
	for _, l := range s.cfg.Leagues {
		leagueSport := s.sportOf(l.Code)
		if sport != "" && leagueSport != sport {
			continue
		}
		rank := max(textMatchRank(term, names.TeamLiteral(l.Name)), textMatchRank(term, strings.ToLower(l.Code)))
		if rank > 0 {
			matches = append(matches, rankedSearchItem{rank: rank, SearchItem: SearchItem{Kind: SearchLeague, ID: l.Code, Name: l.Name, Sport: leagueSport, LeagueCode: l.Code}})
		}
	}
	return matches
}

func textMatchRank(term, normalized string) float64 {
	switch {
	case strings.HasPrefix(normalized, term):
		return prefixMatchRank
	case strings.Contains(normalized, term):
		return substringMatchRank
	}
	return 0
}

const searchEntitiesSQL = `
WITH matched_teams AS (
	SELECT t.id, t.name, t."leagueCode",
		((t."normalizedName" LIKE $1 || '%')::int + word_similarity($1, t."normalizedName"))::float8 AS rank
	FROM teams t
	WHERE $1 <> '' AND (t."normalizedName" LIKE '%' || $1 || '%' OR $1 <% t."normalizedName")
		AND ($3::text[] IS NULL OR t."leagueCode" = ANY($3))
	ORDER BY rank DESC, t.name LIMIT $4
),
matched_players AS (
	SELECT p.id, p.name, t.id AS "teamId", t."leagueCode",
		((p."normalizedName" LIKE $2 || '%')::int + word_similarity($2, p."normalizedName"))::float8 AS rank
	FROM players p JOIN teams t ON t.id = p."teamId"
	WHERE $2 <> '' AND (p."normalizedName" LIKE '%' || $2 || '%' OR $2 <% p."normalizedName")
		AND ($3::text[] IS NULL OR t."leagueCode" = ANY($3))
	ORDER BY rank DESC, p.name LIMIT $4
),
matched_events AS (
	SELECT e.id, h.name || ' vs ' || a.name AS name, e."leagueCode", e.kickoff, e."homeTeamId", e."awayTeamId",
		greatest(mh.rank, ma.rank) - $6 AS rank
	FROM events e
	JOIN teams h ON h.id = e."homeTeamId" JOIN teams a ON a.id = e."awayTeamId"
	LEFT JOIN matched_teams mh ON mh.id = e."homeTeamId" LEFT JOIN matched_teams ma ON ma.id = e."awayTeamId"
	WHERE (mh.id IS NOT NULL OR ma.id IS NOT NULL) AND e.kickoff >= $5 AND e.status IN ('scheduled', 'live') AND e."mergedInto" IS NULL
		AND ($3::text[] IS NULL OR e."leagueCode" = ANY($3))
	ORDER BY rank DESC, e.kickoff LIMIT $4
)
SELECT 'team', id::text, name, "leagueCode", NULL::timestamptz, rank, id::text, '' FROM matched_teams
UNION ALL
SELECT 'player', id::text, name, "leagueCode", NULL::timestamptz, rank, "teamId"::text, '' FROM matched_players
UNION ALL
SELECT 'event', id::text, name, "leagueCode", kickoff, rank, "homeTeamId"::text, "awayTeamId"::text FROM matched_events
ORDER BY 6 DESC, 5 NULLS FIRST, 3
LIMIT $4`

func (s *Server) matchingEntities(ctx context.Context, terms searchTerms, leagueCodes []string, limit int) ([]rankedSearchItem, error) {
	since := time.Now().Add(-recentKickoffGrace)
	rows, err := s.pool.Query(ctx, searchEntitiesSQL, terms.team, terms.person, leagueCodes, limit, since, eventRankDiscount)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rankedSearchItem, error) {
		var item rankedSearchItem
		err := row.Scan(&item.Kind, &item.ID, &item.Name, &item.LeagueCode, &item.Kickoff, &item.rank, &item.logoTeamID, &item.awayLogoTeamID)
		return item, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan search results: %w", err)
	}
	for i := range items {
		items[i].Sport = s.sportOf(items[i].LeagueCode)
	}
	return items, nil
}

func (s *Server) attachSearchLogos(ctx context.Context, items []rankedSearchItem) error {
	var teamIDs, leagueCodes []string
	for _, item := range items {
		switch {
		case item.Kind == SearchLeague:
			leagueCodes = append(leagueCodes, item.ID)
		case item.logoTeamID != "":
			teamIDs = append(teamIDs, item.logoTeamID)
		}
		if item.awayLogoTeamID != "" {
			teamIDs = append(teamIDs, item.awayLogoTeamID)
		}
	}
	teamLogos, err := s.logoURLs(ctx, logos.KindTeam, teamIDs)
	if err != nil {
		return err
	}
	leagueLogos, err := s.logoURLs(ctx, logos.KindLeague, leagueCodes)
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].Kind == SearchLeague {
			items[i].LogoURL = leagueLogos[items[i].ID]
			continue
		}
		items[i].LogoURL = teamLogos[items[i].logoTeamID]
		items[i].AwayLogoURL = teamLogos[items[i].awayLogoTeamID]
	}
	return nil
}
