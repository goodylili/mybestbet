package server

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"mybestbet/pkg/canon"
)

const (
	baysedSource       = "bayse"
	baysedLookback     = 3 * time.Hour
	baysedDefaultHours = 72
	baysedMaxHours     = 14 * 24
	baysedDefaultGap   = 0.05
	baysedMaxAge       = 5 * time.Second
	baysedMarketBatch  = 25
	flagBeatsBooks     = "beats_books"
	flagBeatsHarder    = "beats_harder_line"
	flagOffConsensus   = "off_consensus"
	fairExact          = "exact"
	fairInterpolated   = "interpolated"
	fairPoisson        = "poisson"
	poissonMaxMean     = 500.0
	poissonSteps       = 60
)

type BaysedQuote struct {
	Source   string          `json:"source"`
	Raw      float64         `json:"raw"`
	Form     canon.PriceForm `json:"form"`
	Prob     float64         `json:"prob"`
	DeepLink string          `json:"deepLink,omitempty"`
}

type BaysedSide struct {
	Side       string       `json:"side"`
	Label      string       `json:"label"`
	Bayse      BaysedQuote  `json:"bayse"`
	Best       *BaysedQuote `json:"best,omitempty"`
	Harder     *BaysedQuote `json:"harder,omitempty"`
	HarderLine *float64     `json:"harderLine,omitempty"`
	Fair       *float64     `json:"fair,omitempty"`
	FairModel  string       `json:"fairModel,omitempty"`
	Gap        *float64     `json:"gap,omitempty"`
	Flags      []string     `json:"flags"`
}

type BaysedLadderSelection struct {
	Side  string                 `json:"side"`
	Label string                 `json:"label"`
	Cells map[string]BaysedQuote `json:"cells"`
}

type BaysedLadderLine struct {
	Line       *float64                `json:"line,omitempty"`
	Selections []BaysedLadderSelection `json:"selections"`
}

type BaysedMarket struct {
	EventID    string             `json:"eventId"`
	Sport      canon.Sport        `json:"sport"`
	LeagueCode string             `json:"leagueCode"`
	Home       string             `json:"home"`
	Away       string             `json:"away"`
	Kickoff    time.Time          `json:"kickoff"`
	KeyString  string             `json:"keyString"`
	Market     canon.MarketCode   `json:"market"`
	Line       *float64           `json:"line,omitempty"`
	Period     string             `json:"period,omitempty"`
	PlayerID   string             `json:"playerId,omitempty"`
	Player     string             `json:"player,omitempty"`
	TeamID     string             `json:"teamId,omitempty"`
	Balanced   *float64           `json:"balancedLine,omitempty"`
	Sides      []BaysedSide       `json:"sides"`
	Ladder     []BaysedLadderLine `json:"ladder"`
	Books      []string           `json:"books"`
	Flags      []string           `json:"flags"`
}

type BaysedReport struct {
	Items     []BaysedMarket `json:"items"`
	Compared  int            `json:"compared"`
	Unmatched int            `json:"unmatched"`
	Gap       float64        `json:"gap"`
	Books     []string       `json:"books"`
}

type baysedFilter struct {
	sport canon.Sport
	from  time.Time
	to    time.Time
	gap   float64
	all   bool
}

type ladderPoint struct {
	line float64
	over float64
}

func (s *Server) getBaysed(w http.ResponseWriter, r *http.Request) {
	filter, err := baysedFilterFrom(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	report, err := s.baysedReport(r.Context(), filter)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, report, baysedMaxAge)
}

func baysedFilterFrom(r *http.Request) (baysedFilter, error) {
	sport, err := sportQuery(r)
	if err != nil {
		return baysedFilter{}, err
	}
	hours, err := boundedIntQuery(r, "hours", baysedDefaultHours, 1, baysedMaxHours)
	if err != nil {
		return baysedFilter{}, err
	}
	gap, err := floatQuery(r, "gap")
	if err != nil {
		return baysedFilter{}, err
	}
	if gap < 0 || gap >= 1 {
		return baysedFilter{}, invalidInput("gap", "must be at least 0 and below 1")
	}
	if gap == 0 {
		gap = baysedDefaultGap
	}
	now := time.Now()
	return baysedFilter{
		sport: sport,
		from:  now.Add(-baysedLookback),
		to:    now.Add(time.Duration(hours) * time.Hour),
		gap:   gap,
		all:   r.URL.Query().Get("all") == "true",
	}, nil
}

func (s *Server) baysedReport(ctx context.Context, filter baysedFilter) (BaysedReport, error) {
	events, err := s.indexedEventsBetween(ctx, eventFilter{leagueCodes: s.leagueCodesOf(filter.sport), from: filter.from, to: filter.to})
	if err != nil {
		return BaysedReport{}, err
	}
	byID := make(map[string]EventInfo, len(events))
	ids := make([]string, len(events))
	for i, event := range events {
		byID[event.ID] = event
		ids[i] = event.ID
	}
	bayseKeys, err := s.sourceMarketKeys(ctx, baysedSource, ids)
	if err != nil {
		return BaysedReport{}, err
	}
	keys, err := s.familyMarketKeys(ctx, bayseKeys)
	if err != nil {
		return BaysedReport{}, err
	}
	cells, err := s.boardCells(ctx, keys)
	if err != nil {
		return BaysedReport{}, err
	}
	now := time.Now()
	families := map[string][]marketGroup{}
	for _, group := range groupByMarket(cells) {
		if event, ok := byID[group.key.EventID]; ok {
			group.cells = pricedSinceKickoff(group.cells, event.Kickoff, now)
			families[familyOf(group.key)] = append(families[familyOf(group.key)], group)
		}
	}
	report := BaysedReport{Items: []BaysedMarket{}, Gap: filter.gap, Books: []string{}}
	for _, family := range families {
		for _, group := range family {
			market, ok := compareBayse(byID[group.key.EventID], group, family, filter.gap)
			if !ok {
				continue
			}
			if len(market.Books) == 0 {
				report.Unmatched++
			} else {
				report.Compared++
			}
			for _, book := range market.Books {
				if !slices.Contains(report.Books, book) {
					report.Books = append(report.Books, book)
				}
			}
			if filter.all || len(market.Flags) > 0 {
				report.Items = append(report.Items, market)
			}
		}
	}
	slices.Sort(report.Books)
	slices.SortFunc(report.Items, compareBaysedMarkets)
	return report, s.nameBaysedPlayers(ctx, report.Items)
}

func familyOf(key canon.MarketKey) string {
	key.Line = nil
	return key.String()
}

func (s *Server) familyMarketKeys(ctx context.Context, bayseKeys []string) ([]string, error) {
	families := map[string]bool{}
	var eventIDs []string
	for _, raw := range bayseKeys {
		key, err := canon.ParseMarketKey(raw)
		if err != nil {
			continue
		}
		families[familyOf(key)] = true
		if !slices.Contains(eventIDs, key.EventID) {
			eventIDs = append(eventIDs, key.EventID)
		}
	}
	var keys []string
	for batch := range slices.Chunk(eventIDs, baysedMarketBatch) {
		markets, err := s.loadEventMarkets(ctx, batch)
		if err != nil {
			return nil, err
		}
		for _, id := range batch {
			for _, raw := range markets[id] {
				if key, err := canon.ParseMarketKey(raw); err == nil && families[familyOf(key)] {
					keys = append(keys, raw)
				}
			}
		}
	}
	return keys, nil
}

func (s *Server) sourceMarketKeys(ctx context.Context, source string, eventIDs []string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT "marketKey" FROM prices_current WHERE source = $1 AND "eventId" = ANY($2::uuid[])`, source, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("query %s markets: %w", source, err)
	}
	all, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("scan %s markets: %w", source, err)
	}
	return all, nil
}

func compareBayse(event EventInfo, group marketGroup, family []marketGroup, gap float64) (BaysedMarket, bool) {
	market := BaysedMarket{
		EventID:    event.ID,
		Sport:      event.Sport,
		LeagueCode: event.LeagueCode,
		Home:       event.Home.Name,
		Away:       event.Away.Name,
		Kickoff:    event.Kickoff,
		KeyString:  group.keyString,
		Market:     group.key.Code,
		Line:       group.key.Line,
		Period:     group.key.Period,
		PlayerID:   group.key.PlayerID,
		TeamID:     group.key.TeamID,
		Books:      []string{},
		Flags:      []string{},
	}
	exactFair := booksFair(group.cells)
	curve := overCurve(family)
	if balanced, ok := balancedLine(curve); ok {
		market.Balanced = &balanced
	}
	for _, selection := range selectionRows(group.cells) {
		bayse, ok := selection.Cells[baysedSource]
		if !ok || !isBettable(bayse) {
			continue
		}
		side := BaysedSide{Side: selection.Side, Label: selection.Label, Bayse: baysedQuote(bayse), Flags: []string{}}
		var others []canon.BoardCell
		for source, cell := range selection.Cells {
			if source != baysedSource && isBettable(cell) {
				others = append(others, cell)
			}
		}
		if best, ok := bestCell(others); ok {
			quote := baysedQuote(best)
			side.Best = &quote
			if bayse.Price.Prob < best.Price.Prob {
				side.Flags = append(side.Flags, flagBeatsBooks)
			}
		}
		if harder, line, ok := harderLineQuote(group.key, selection.Side, bayse, family); ok {
			side.Harder, side.HarderLine = &harder, &line
			side.Flags = append(side.Flags, flagBeatsHarder)
		}
		if p, model, ok := sideFair(group.key, selection, exactFair, curve); ok {
			diff := p - bayse.Price.Prob
			side.Fair, side.Gap, side.FairModel = &p, &diff, model
			if diff >= gap || -diff >= gap {
				side.Flags = append(side.Flags, flagOffConsensus)
			}
		}
		for _, flag := range side.Flags {
			if !slices.Contains(market.Flags, flag) {
				market.Flags = append(market.Flags, flag)
			}
		}
		market.Sides = append(market.Sides, side)
	}
	if len(market.Sides) == 0 {
		return BaysedMarket{}, false
	}
	offered := map[string]bool{}
	for _, side := range market.Sides {
		offered[side.Side+"|"+side.Label] = true
	}
	market.Ladder, market.Books = baysedLadder(family, offered)
	return market, true
}

func sideFair(key canon.MarketKey, selection SelectionRow, exact map[string]float64, curve []ladderPoint) (float64, string, bool) {
	if p, ok := exact[selection.Side+"|"+selection.Label]; ok {
		return p, fairExact, true
	}
	over := canon.Side(selection.Side) == canon.SideOver
	if !halfLine(key.Line) || (!over && canon.Side(selection.Side) != canon.SideUnder) {
		return 0, "", false
	}
	p, model := 0.0, fairInterpolated
	if interpolated, ok := interpolateOver(curve, *key.Line); ok {
		p = interpolated
	} else if modelled, ok := poissonOver(curve, *key.Line); ok {
		p, model = modelled, fairPoisson
	} else {
		return 0, "", false
	}
	if over {
		return p, model, true
	}
	return 1 - p, model, true
}

func balancedLine(curve []ladderPoint) (float64, bool) {
	if len(curve) == 0 {
		return 0, false
	}
	best := curve[0]
	for _, point := range curve[1:] {
		if math.Abs(point.over-0.5) < math.Abs(best.over-0.5) {
			best = point
		}
	}
	return best.line, true
}

func poissonOver(curve []ladderPoint, line float64) (float64, bool) {
	if line < 0 || len(curve) == 0 {
		return 0, false
	}
	anchor := curve[0]
	for _, point := range curve[1:] {
		if math.Abs(point.over-0.5) < math.Abs(anchor.over-0.5) {
			anchor = point
		}
	}
	if anchor.line < 0 || anchor.over <= 0 || anchor.over >= 1 {
		return 0, false
	}
	low, high := 0.0, poissonMaxMean
	for range poissonSteps {
		mean := (low + high) / 2
		if poissonTail(mean, anchor.line) < anchor.over {
			low = mean
		} else {
			high = mean
		}
	}
	return poissonTail((low+high)/2, line), true
}

func poissonTail(mean, line float64) float64 {
	term := math.Exp(-mean)
	below := term
	for k := 1; float64(k) <= math.Floor(line); k++ {
		term *= mean / float64(k)
		below += term
	}
	return max(0, 1-below)
}

func overCurve(family []marketGroup) []ladderPoint {
	var curve []ladderPoint
	for _, group := range family {
		if !halfLine(group.key.Line) {
			continue
		}
		fair := booksFair(group.cells)
		if over, ok := fair[string(canon.SideOver)+"|"]; ok {
			curve = append(curve, ladderPoint{line: *group.key.Line, over: over})
		}
	}
	slices.SortFunc(curve, func(a, b ladderPoint) int { return cmp.Compare(a.line, b.line) })
	return curve
}

func interpolateOver(curve []ladderPoint, line float64) (float64, bool) {
	for i := 1; i < len(curve); i++ {
		low, high := curve[i-1], curve[i]
		if line > low.line && line < high.line {
			weight := (line - low.line) / (high.line - low.line)
			return low.over + weight*(high.over-low.over), true
		}
	}
	return 0, false
}

func harderLineQuote(key canon.MarketKey, side string, bayse canon.BoardCell, family []marketGroup) (BaysedQuote, float64, bool) {
	if !halfLine(key.Line) {
		return BaysedQuote{}, 0, false
	}
	over := canon.Side(side) == canon.SideOver
	if !over && canon.Side(side) != canon.SideUnder {
		return BaysedQuote{}, 0, false
	}
	var found canon.BoardCell
	var foundLine float64
	for _, group := range family {
		if !halfLine(group.key.Line) {
			continue
		}
		line := *group.key.Line
		harder := (over && line > *key.Line) || (!over && line < *key.Line)
		if !harder {
			continue
		}
		for _, cell := range group.cells {
			beaten := cell.Source != baysedSource && string(cell.Side) == side && isBettable(cell) && bayse.Price.Prob < cell.Price.Prob
			if beaten && cell.Price.Prob > found.Price.Prob {
				found, foundLine = cell, line
			}
		}
	}
	if found.Source == "" {
		return BaysedQuote{}, 0, false
	}
	return baysedQuote(found), foundLine, true
}

func halfLine(line *float64) bool {
	return line != nil && math.Abs(math.Mod(*line, 1)) == 0.5
}

func baysedLadder(family []marketGroup, offered map[string]bool) ([]BaysedLadderLine, []string) {
	books := []string{}
	ladder := make([]BaysedLadderLine, 0, len(family))
	for _, group := range family {
		line := BaysedLadderLine{Line: group.key.Line}
		for _, selection := range selectionRows(group.cells) {
			if !offered[selection.Side+"|"+selection.Label] {
				continue
			}
			cells := map[string]BaysedQuote{}
			for source, cell := range selection.Cells {
				if !isBettable(cell) {
					continue
				}
				cells[source] = baysedQuote(cell)
				if source != baysedSource && !slices.Contains(books, source) {
					books = append(books, source)
				}
			}
			if len(cells) > 0 {
				line.Selections = append(line.Selections, BaysedLadderSelection{Side: selection.Side, Label: selection.Label, Cells: cells})
			}
		}
		if len(line.Selections) > 0 {
			ladder = append(ladder, line)
		}
	}
	slices.SortFunc(ladder, func(a, b BaysedLadderLine) int {
		return cmp.Compare(lineValue(a.Line), lineValue(b.Line))
	})
	slices.Sort(books)
	return ladder, books
}

func lineValue(line *float64) float64 {
	if line == nil {
		return 0
	}
	return *line
}

func booksFair(cells []canon.BoardCell) map[string]float64 {
	selections := map[string]bool{}
	bySource := map[string][]canon.BoardCell{}
	for _, cell := range cells {
		if cell.Source == consensusSource {
			continue
		}
		selections[string(cell.Side)+"|"+cell.Label] = true
		if cell.Source != baysedSource && isBettable(cell) {
			bySource[cell.Source] = append(bySource[cell.Source], cell)
		}
	}
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, priced := range bySource {
		if len(priced) != len(selections) || len(priced) < 2 {
			continue
		}
		var total float64
		for _, cell := range priced {
			total += cell.Price.Prob
		}
		for _, cell := range priced {
			id := string(cell.Side) + "|" + cell.Label
			sums[id] += cell.Price.Prob / total
			counts[id]++
		}
	}
	fair := make(map[string]float64, len(sums))
	for id, sum := range sums {
		fair[id] = sum / float64(counts[id])
	}
	return fair
}

func baysedQuote(cell canon.BoardCell) BaysedQuote {
	return BaysedQuote{Source: cell.Source, Raw: cell.Price.Raw, Form: cell.Price.Form, Prob: cell.Price.Prob, DeepLink: cell.DeepLink}
}

func compareBaysedMarkets(a, b BaysedMarket) int {
	return cmp.Or(
		cmp.Compare(len(b.Flags), len(a.Flags)),
		cmp.Compare(baysedEdge(b), baysedEdge(a)),
		a.Kickoff.Compare(b.Kickoff),
		cmp.Compare(a.KeyString, b.KeyString),
	)
}

func baysedEdge(market BaysedMarket) float64 {
	var edge float64
	for _, side := range market.Sides {
		if side.Gap != nil {
			edge = max(edge, *side.Gap, -*side.Gap)
		}
	}
	return edge
}

func (s *Server) nameBaysedPlayers(ctx context.Context, markets []BaysedMarket) error {
	seen := map[string]bool{}
	var ids []string
	for _, market := range markets {
		if market.PlayerID != "" && !seen[market.PlayerID] {
			seen[market.PlayerID] = true
			ids = append(ids, market.PlayerID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	players, err := s.playersByID(ctx, ids)
	if err != nil {
		return err
	}
	for i := range markets {
		player := players[markets[i].PlayerID]
		markets[i].Player = player.Name
		if markets[i].TeamID == "" {
			markets[i].TeamID = player.TeamID
		}
	}
	return nil
}
