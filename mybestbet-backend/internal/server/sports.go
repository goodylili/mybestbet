package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

func sportByLeague(leagues []config.League) map[string]canon.Sport {
	sports := make(map[string]canon.Sport, len(leagues))
	for _, l := range leagues {
		sports[l.Code] = l.Sport
	}
	return sports
}

const leagueSportRefresh = time.Minute

type storedLeague struct {
	code  string
	sport canon.Sport
}

func (s *Server) sportOf(leagueCode string) canon.Sport {
	if sport := s.leagueSports[leagueCode]; sport != "" {
		return sport
	}
	if stored := s.storedLeagues.Load(); stored != nil {
		if sport := (*stored)[leagueCode]; sport != "" {
			return sport
		}
	}
	return canon.SportFootball
}

func (s *Server) refreshLeagueSports(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT code, sport FROM leagues`)
	if err != nil {
		return fmt.Errorf("list league sports: %w", err)
	}
	stored := map[string]canon.Sport{}
	var league storedLeague
	_, err = pgx.ForEachRow(rows, []any{&league.code, &league.sport}, func() error {
		if _, configured := s.leagueSports[league.code]; !configured {
			stored[league.code] = league.sport
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("list league sports: %w", err)
	}
	s.storedLeagues.Store(&stored)
	return nil
}

func (s *Server) withSports(events []EventInfo) []EventInfo {
	for i := range events {
		events[i].Sport = s.sportOf(events[i].LeagueCode)
	}
	return events
}

func (s *Server) leagueCodesOf(sport canon.Sport) []string {
	if sport == "" {
		return nil
	}
	codes := []string{}
	for _, l := range s.cfg.Leagues {
		if s.sportOf(l.Code) == sport {
			codes = append(codes, l.Code)
		}
	}
	stored := s.storedLeagues.Load()
	if stored == nil {
		return codes
	}
	extra := []string{}
	for code, storedSport := range *stored {
		if storedSport == sport {
			extra = append(extra, code)
		}
	}
	slices.Sort(extra)
	return append(codes, extra...)
}

func sportNames() []string {
	names := make([]string, len(canon.Sports))
	for i, sport := range canon.Sports {
		names[i] = string(sport)
	}
	return names
}

func sportQuery(r *http.Request) (canon.Sport, error) {
	sport, err := enumQuery(r, "sport", sportNames()...)
	return canon.Sport(sport), err
}

func mainMarketOf(sport canon.Sport) canon.MarketCode {
	if sport == canon.SportFootball {
		return canon.Code1X2
	}
	return canon.CodeMoneyline
}

func (s *Server) leaguesWithCounts(r *http.Request, sport canon.Sport) ([]League, error) {
	all, ok := s.leagues.get()
	if !ok {
		counts, err := s.fixtureCountsByLeague(r.Context(), time.Now().Add(-recentKickoffGrace))
		if err != nil {
			return nil, err
		}
		if all, err = s.loadLeagues(r.Context(), counts); err != nil {
			return nil, err
		}
	}
	leagues := []League{}
	for _, l := range all {
		if sport == "" || l.Sport == sport {
			leagues = append(leagues, l)
		}
	}
	return leagues, nil
}

func (s *Server) loadLeagues(ctx context.Context, counts map[string]int) ([]League, error) {
	leagues := make([]League, len(s.cfg.Leagues))
	for i, l := range s.cfg.Leagues {
		leagues[i] = League{Code: l.Code, Name: l.Name, Country: l.Country, Sport: s.sportOf(l.Code), Group: l.Group, Order: l.Order, FixtureCount: counts[l.Code]}
	}
	return leagues, s.attachLeagueLogos(ctx, leagues)
}

func (s *Server) listLeagues(w http.ResponseWriter, r *http.Request) {
	sport, err := sportQuery(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	leagues, err := s.leaguesWithCounts(r, sport)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, listOf(leagues), sportsMaxAge)
}

func (s *Server) listSports(w http.ResponseWriter, r *http.Request) {
	leagues, err := s.leaguesWithCounts(r, "")
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, listOf(sportSummaries(leagues)), sportsMaxAge)
}

func sportSummaries(leagues []League) []SportSummary {
	var summaries []SportSummary
	for _, sport := range canon.Sports {
		summary := SportSummary{
			Sport:        sport,
			Leagues:      []League{},
			MatchMarkets: canon.MarketsFor(sport, canon.KindMatch),
			PlayerProps:  emptyIfNil(canon.MarketsFor(sport, canon.KindPlayer)),
			Markets:      marketsOf(sport),
		}
		for _, l := range leagues {
			if l.Sport == sport {
				summary.Leagues = append(summary.Leagues, l)
				summary.FixtureCount += l.FixtureCount
			}
		}
		if len(summary.Leagues) > 0 {
			summaries = append(summaries, summary)
		}
	}
	return summaries
}
