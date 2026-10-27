package canon

import (
	"math"
	"time"
)

const (
	maxTradableShareSpread = 0.10
	unitsPerShare          = 100
	shareUnitPrecision     = 1e4
)

func IsQuotedShare(share float64) bool { return share > 0 && share < 1 }

func ShareIn(form PriceForm, share float64) float64 {
	switch form {
	case FormCents, FormNaira:
		return math.Round(share*unitsPerShare*shareUnitPrecision) / shareUnitPrecision
	case FormDecimal, FormShare, FormLineOnly:
	}
	return share
}

func NewShareBookPrice(bid, ask, askSize float64, form PriceForm, observedAt time.Time) Price {
	var bidIn, askIn, liquidity *float64
	if IsQuotedShare(bid) {
		bidIn = new(ShareIn(form, bid))
	}
	if IsQuotedShare(ask) {
		askIn = new(ShareIn(form, ask))
		liquidity = new(ask * askSize)
	}
	p := NewBookPrice(bidIn, askIn, liquidity, form, observedAt)
	p.Suspended = !isTradableBook(bid, ask, askSize)
	return p
}

func isTradableBook(bid, ask, askSize float64) bool {
	return IsQuotedShare(bid) && IsQuotedShare(ask) && askSize > 0 && ask-bid <= maxTradableShareSpread
}
