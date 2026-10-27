package kalshi

import (
	"time"

	"mybestbet/pkg/canon"
)

type outcome int

const (
	outcomeYes outcome = iota + 1
	outcomeNo
)

type quote struct {
	yesBid     float64
	yesAsk     float64
	yesBidSize float64
	yesAskSize float64
}

func (q quote) price(o outcome, observedAt time.Time) canon.Price {
	if o == outcomeNo {
		return canon.NewShareBookPrice(complement(q.yesAsk), complement(q.yesBid), q.yesBidSize, canon.FormCents, observedAt)
	}
	return canon.NewShareBookPrice(q.yesBid, q.yesAsk, q.yesAskSize, canon.FormCents, observedAt)
}

func (q quote) withUpdate(msg tickerMsg) quote {
	overwrite := func(dst *float64, src *decimal) {
		if src != nil {
			*dst = float64(*src)
		}
	}
	overwrite(&q.yesBid, msg.YesBid)
	overwrite(&q.yesAsk, msg.YesAsk)
	overwrite(&q.yesBidSize, msg.YesBidSize)
	overwrite(&q.yesAskSize, msg.YesAskSize)
	return q
}

func complement(dollars float64) float64 {
	if !canon.IsQuotedShare(dollars) {
		return 0
	}
	return 1 - dollars
}
