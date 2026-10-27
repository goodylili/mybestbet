package bayse

import (
	"time"

	"mybestbet/pkg/canon"
)

func ammPrice(share float64, untraded bool, observedAt time.Time) canon.Price {
	p := canon.NewPrice(canon.ShareIn(canon.FormNaira, share), canon.FormNaira, observedAt)
	p.Suspended = untraded || !canon.IsQuotedShare(share)
	return p
}

type quote struct {
	bid     float64
	ask     float64
	askSize float64
}

func quoteOf(book outcomeBook) quote {
	var q quote
	for _, lvl := range book.Bids {
		if lvl.Quantity > 0 && lvl.Price > q.bid {
			q.bid = lvl.Price
		}
	}
	for _, lvl := range book.Asks {
		if lvl.Quantity > 0 && (q.ask == 0 || lvl.Price < q.ask) {
			q.ask, q.askSize = lvl.Price, lvl.Quantity
		}
	}
	return q
}

func (q quote) price(observedAt time.Time) canon.Price {
	return canon.NewShareBookPrice(q.bid, q.ask, q.askSize, canon.FormNaira, observedAt)
}
