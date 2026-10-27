package server

import (
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/stats"
	"mybestbet/pkg/canon"
)

const (
	defaultSignalLimit = 200
	maxSignalLimit     = 500
	defaultStatsLast   = 20
	maxStatsLast       = 100
)

func (s *Server) listSignals(w http.ResponseWriter, r *http.Request) {
	filter, err := s.signalFilterFrom(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	signals, err := s.cachedSignals(r.Context(), filter)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, listOf(signals), signalsMaxAge)
}

func (s *Server) signalFilterFrom(r *http.Request) (signalFilter, error) {
	signalType, err := enumQuery(r, "type", string(canon.SignalArb), string(canon.SignalEV), string(canon.SignalLine), string(canon.SignalMove))
	if err != nil {
		return signalFilter{}, err
	}
	minimum, err := floatQuery(r, "min")
	if err != nil {
		return signalFilter{}, err
	}
	limit, err := boundedIntQuery(r, "limit", defaultSignalLimit, 1, maxSignalLimit)
	if err != nil {
		return signalFilter{}, err
	}
	sport, err := sportQuery(r)
	if err != nil {
		return signalFilter{}, err
	}
	return signalFilter{signalType: signalType, leagueCodes: s.leagueCodesOf(sport), minimum: minimum, limit: limit}, nil
}

func (s *Server) getMarketHistory(w http.ResponseWriter, r *http.Request) {
	key, err := canon.ParseMarketKey(r.PathValue("key"))
	if err != nil {
		s.writeFailure(w, r, invalidInput("key", "must be eventId|code|line|playerId|period"))
		return
	}
	if uuid.Validate(key.EventID) != nil {
		s.writeFailure(w, r, invalidInput("key", "event id must be a uuid"))
		return
	}
	points, err := s.marketHistory(r.Context(), key, r.URL.Query().Get("source"))
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MarketHistory{MarketKey: key.String(), Points: emptyIfNil(points)})
}

func (s *Server) getPlayerStats(w http.ResponseWriter, r *http.Request) {
	id, err := uuidPathValue(r, "id")
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	last, err := boundedIntQuery(r, "last", defaultStatsLast, 1, maxStatsLast)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	var player PropPlayer
	var matches []stats.MatchStat
	group, groupCtx := errgroup.WithContext(r.Context())
	group.Go(func() (err error) {
		player, err = s.playerByID(groupCtx, id)
		return err
	})
	group.Go(func() (err error) {
		matches, err = s.stats.PlayerStats(groupCtx, id, last)
		return err
	})
	if err := group.Wait(); err != nil {
		s.writeFailure(w, r, err)
		return
	}
	items, err := s.withOpponents(r.Context(), matchStatsDTO(matches))
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, PlayerStats{Player: player, Items: items})
}
