package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"

	"mybestbet/internal/stats"
	"mybestbet/pkg/canon"
)

const yesNoStatLine = 0.5

func (s *Server) getEventProps(w http.ResponseWriter, r *http.Request) {
	event, cells, err := s.eventWithBoard(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	board, err := s.propBoard(r.Context(), event, cells)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, board, eventMaxAge)
}

func (s *Server) propBoard(ctx context.Context, event EventInfo, cells []canon.BoardCell) (PropBoard, error) {
	var rows []PropRow
	var keys []canon.MarketKey
	var propCells []canon.BoardCell
	for _, group := range groupByMarket(cells) {
		if group.key.PlayerID == "" {
			continue
		}
		rows = append(rows, PropRow{
			KeyString:  group.keyString,
			Selections: propSelections(selectionRows(group.cells)),
		})
		keys = append(keys, group.key)
		propCells = append(propCells, group.cells...)
	}
	players, err := s.propPlayers(ctx, event, rows, keys)
	if err != nil {
		return PropBoard{}, err
	}
	return PropBoard{Event: event, Sources: sourceColumns(propCells), Players: players, Rows: emptyIfNil(rows)}, nil
}

func propSelections(rows []SelectionRow) []PropSelection {
	out := make([]PropSelection, len(rows))
	for i, row := range rows {
		cells := make(map[string]PropCell, len(row.Cells))
		for source, cell := range row.Cells {
			cells[source] = propCell(cell)
		}
		out[i] = PropSelection{Side: row.Side, Label: row.Label, BestSource: row.BestSource, Cells: cells}
		if row.Consensus != nil {
			consensus := propCell(*row.Consensus)
			out[i].Consensus = &consensus
		}
	}
	return out
}

func propCell(cell canon.BoardCell) PropCell {
	price := cell.Price
	out := PropCell{
		Raw:       price.Raw,
		Bid:       price.Bid,
		Ask:       price.Ask,
		Liquidity: price.Liquidity,
		Suspended: price.Suspended,
		Fair:      cell.Fair,
		Derived:   cell.Derived,
		DeepLink:  cell.DeepLink,
	}
	if price.Form != canon.FormDecimal {
		out.Form = price.Form
	}
	if price.Prob != canon.ImpliedProb(price.Raw, price.Form) {
		out.Prob = &price.Prob
	}
	return out
}

func (s *Server) propPlayers(ctx context.Context, event EventInfo, rows []PropRow, keys []canon.MarketKey) ([]PropPlayerInfo, error) {
	playerIDs := distinctPlayerIDs(keys)
	if len(playerIDs) == 0 {
		return []PropPlayerInfo{}, nil
	}
	var players map[string]PropPlayer
	var histories map[string]stats.History
	var lineups map[string]stats.LineupStatus
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() (err error) {
		players, err = s.playersByID(groupCtx, playerIDs)
		return err
	})
	group.Go(func() (err error) {
		if histories, err = s.stats.Histories(groupCtx, playerIDs); err != nil {
			return fmt.Errorf("player histories: %w", err)
		}
		return nil
	})
	group.Go(func() (err error) {
		if lineups, err = s.stats.LineupStatuses(groupCtx, event.ID, playerIDs); err != nil {
			return fmt.Errorf("lineup statuses: %w", err)
		}
		return nil
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}
	infos := make([]PropPlayerInfo, len(playerIDs))
	index := make(map[string]int, len(playerIDs))
	for i, id := range playerIDs {
		player, ok := players[id]
		if !ok {
			player = PropPlayer{ID: id}
		}
		lineup := lineups[id]
		index[id] = i
		infos[i] = PropPlayerInfo{
			PropPlayer:   player,
			MinutesTrend: emptyIfNil(minutesDTO(histories[id].MinutesTrend())),
			LineupStatus: string(lineup),
			BenchWarning: lineup == stats.LineupBench,
		}
	}
	for i := range rows {
		info := &infos[index[keys[i].PlayerID]]
		history := histories[keys[i].PlayerID]
		research, err := rowResearch(event, keys[i], info.TeamID, history)
		if err != nil {
			s.log.WarnContext(ctx, "prop research unavailable", "event_id", event.ID, "market_key", rows[i].KeyString, "err", err)
			continue
		}
		rows[i].Research = research
		if research != nil && keys[i].Code == canon.PropRating && info.Ratings == nil {
			info.Ratings = ratingDistributionDTO(history.RatingDistribution())
		}
	}
	return infos, nil
}

func distinctPlayerIDs(keys []canon.MarketKey) []string {
	seen := map[string]bool{}
	var ids []string
	for _, key := range keys {
		if !seen[key.PlayerID] {
			seen[key.PlayerID] = true
			ids = append(ids, key.PlayerID)
		}
	}
	return ids
}

func rowResearch(event EventInfo, key canon.MarketKey, playerTeamID string, history stats.History) (*PropResearch, error) {
	line := statLine(key)
	rates, err := history.HitRates(key.Code, line, nil)
	if errors.Is(err, stats.ErrUnsupportedCode) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hit rates: %w", err)
	}
	research := &PropResearch{
		Windows: hitRatesDTO(rates.Windows),
		Home:    hitRateDTO(rates.Home),
		Away:    hitRateDTO(rates.Away),
	}
	if opponent, ok := opponentOf(event, playerTeamID); ok {
		vsRate, vsMatches, err := history.VsOpponent(opponent.ID, key.Code, line)
		if err != nil {
			return nil, fmt.Errorf("vs opponent: %w", err)
		}
		research.VsOpponent = hitRateDTO(vsRate)
		research.VsOpponentMatches = namedOpponents(matchStatsDTO(vsMatches), map[string]Team{opponent.ID: opponent})
	}
	return research, nil
}

func statLine(key canon.MarketKey) float64 {
	if key.Line == nil {
		return yesNoStatLine
	}
	return *key.Line
}

func opponentOf(event EventInfo, playerTeamID string) (Team, bool) {
	switch playerTeamID {
	case event.Home.ID:
		return event.Away, true
	case event.Away.ID:
		return event.Home, true
	}
	return Team{}, false
}
