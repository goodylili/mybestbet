package polymarket

import (
	"time"

	"mybestbet/pkg/canon"
)

const (
	sideBuy  = "BUY"
	sideSell = "SELL"
)

type ladder struct {
	bids map[float64]float64
	asks map[float64]float64
}

func newLadder(bids, asks []bookLevel) *ladder {
	l := &ladder{bids: map[float64]float64{}, asks: map[float64]float64{}}
	for _, lvl := range bids {
		l.set(sideBuy, float64(lvl.Price), float64(lvl.Size))
	}
	for _, lvl := range asks {
		l.set(sideSell, float64(lvl.Price), float64(lvl.Size))
	}
	return l
}

func (l *ladder) set(side string, price, size float64) {
	levels := l.asks
	if side == sideBuy {
		levels = l.bids
	}
	if size <= 0 {
		delete(levels, price)
		return
	}
	levels[price] = size
}

func (l *ladder) touchPrice(observedAt time.Time) canon.Price {
	var bid, ask float64
	for p := range l.bids {
		bid = max(bid, p)
	}
	for p := range l.asks {
		if ask == 0 || p < ask {
			ask = p
		}
	}
	return canon.NewShareBookPrice(bid, ask, l.asks[ask], canon.FormShare, observedAt)
}

func missingBookPrice(observedAt time.Time) canon.Price {
	return canon.Price{Form: canon.FormShare, Suspended: true, ObservedAt: observedAt}
}
