package booking

import (
	"cmp"
	"math"
	"slices"

	"mybestbet/pkg/canon"
)

const consensusSource = "consensus"

func decimalOdds(cell canon.BoardCell) float64 {
	price := cell.Price
	if cell.Derived || price.Suspended || cell.Source == consensusSource {
		return 0
	}
	if price.Form == canon.FormDecimal {
		if price.Raw > 1 {
			return price.Raw
		}
		return 0
	}
	if price.Prob > 0 && price.Prob < 1 {
		return 1 / price.Prob
	}
	return 0
}

func (d slipData) odds(leg slipLeg, source string) float64 {
	cell, ok := d.cell(leg, source)
	if !ok {
		return 0
	}
	return decimalOdds(cell)
}

func (s *Service) compare(legs []slipLeg, data slipData, stake float64) Comparison {
	comparison := Comparison{
		Stake:     stake,
		TotalLegs: len(legs),
		Picked:    newSplitQuote(),
		BestSplit: newSplitQuote(),
		Books:     make([]BookQuote, 0, len(s.books)),
	}
	for _, leg := range legs {
		comparison.Picked.add(leg.index, leg.source, data.odds(leg, leg.source))
		source, price := s.bestPrice(leg, data)
		comparison.BestSplit.add(leg.index, source, price)
	}
	comparison.Picked.finish(stake)
	comparison.BestSplit.finish(stake)
	for _, book := range s.books {
		comparison.Books = append(comparison.Books, quoteBook(book, legs, data, stake))
	}
	slices.SortStableFunc(comparison.Books, func(a, b BookQuote) int {
		return cmp.Or(cmp.Compare(b.Covered, a.Covered), cmp.Compare(b.Return, a.Return), cmp.Compare(a.Book, b.Book))
	})
	return comparison
}

func (s *Service) bestPrice(leg slipLeg, data slipData) (string, float64) {
	var bestSource, fallbackSource string
	var best, fallback float64
	for field, cell := range data.cells[leg.keyStr] {
		price := decimalOdds(cell)
		if price == 0 || field != fieldOf(cell.Source, leg) {
			continue
		}
		if s.bookers[cell.Source] != nil {
			if price > best || (price == best && cell.Source < bestSource) {
				bestSource, best = cell.Source, price
			}
			continue
		}
		if price > fallback || (price == fallback && cell.Source < fallbackSource) {
			fallbackSource, fallback = cell.Source, price
		}
	}
	if best > 0 {
		return bestSource, best
	}
	return fallbackSource, fallback
}

func quoteBook(book bookInfo, legs []slipLeg, data slipData, stake float64) BookQuote {
	quote := BookQuote{Book: book.name, Bookable: book.booker != nil, Total: len(legs), Legs: []QuoteLeg{}, Missing: []int{}}
	split := newSplitQuote()
	for _, leg := range legs {
		price := data.odds(leg, book.name)
		if price == 0 {
			quote.Missing = append(quote.Missing, leg.index)
			continue
		}
		split.add(leg.index, book.name, price)
	}
	split.finish(stake)
	quote.Covered, quote.Odds, quote.Return, quote.Legs = split.Covered, split.Odds, split.Return, split.Legs
	return quote
}

func newSplitQuote() SplitQuote { return SplitQuote{Odds: 1, Legs: []QuoteLeg{}} }

func (q *SplitQuote) add(index int, source string, price float64) {
	if price <= 1 {
		return
	}
	q.Covered++
	q.Odds *= price
	q.Legs = append(q.Legs, QuoteLeg{Index: index, Source: source, Price: price})
}

func (q *SplitQuote) finish(stake float64) {
	if q.Covered == 0 {
		q.Odds = 0
	}
	q.Odds = round(q.Odds, 4)
	q.Return = round(q.Odds*stake, 2)
}

func round(v float64, places int) float64 {
	scale := math.Pow10(places)
	return math.Round(v*scale) / scale
}
