package predictfun

import (
	"math"
	"time"

	"mybestbet/pkg/canon"
)

type outcomeSide int

const (
	outcomeYes outcomeSide = iota + 1
	outcomeNo
)

const (
	levelPrice     = 0
	levelQuantity  = 1
	sharePrecision = 1e6
)

type quote struct {
	yesBid     float64
	yesAsk     float64
	yesBidSize float64
	yesAskSize float64
}

func quoteOf(book orderbook) quote {
	var q quote
	for _, lvl := range book.Bids {
		if lvl[levelQuantity] > 0 && lvl[levelPrice] > q.yesBid {
			q.yesBid, q.yesBidSize = lvl[levelPrice], lvl[levelQuantity]
		}
	}
	for _, lvl := range book.Asks {
		if lvl[levelQuantity] > 0 && (q.yesAsk == 0 || lvl[levelPrice] < q.yesAsk) {
			q.yesAsk, q.yesAskSize = lvl[levelPrice], lvl[levelQuantity]
		}
	}
	return q
}

const yesIndexSet = 1

func inlineQuoteOf(m market) (quote, bool) {
	for i, o := range m.Outcomes {
		isYes := o.IndexSet == yesIndexSet || (o.IndexSet == 0 && i == firstOutcome)
		if !isYes || (o.BestBid == nil && o.BestAsk == nil) {
			continue
		}
		var q quote
		if o.BestBid != nil && o.BestBid.Size > 0 {
			q.yesBid, q.yesBidSize = o.BestBid.Price, o.BestBid.Size
		}
		if o.BestAsk != nil && o.BestAsk.Size > 0 {
			q.yesAsk, q.yesAskSize = o.BestAsk.Price, o.BestAsk.Size
		}
		return q, true
	}
	return quote{}, false
}

func (q quote) price(o outcomeSide, observedAt time.Time) canon.Price {
	if o == outcomeNo {
		return canon.NewShareBookPrice(complement(q.yesAsk), complement(q.yesBid), q.yesBidSize, canon.FormShare, observedAt)
	}
	return canon.NewShareBookPrice(q.yesBid, q.yesAsk, q.yesAskSize, canon.FormShare, observedAt)
}

func complement(share float64) float64 {
	if !canon.IsQuotedShare(share) {
		return 0
	}
	return math.Round((1-share)*sharePrecision) / sharePrecision
}
