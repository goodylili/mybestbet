package loadtest

import (
	"math/rand/v2"
	"slices"
	"time"

	"mybestbet/pkg/canon"
)

func repriced(m canon.Market, observedAt time.Time, maxJitter float64) canon.Market {
	m.Selections = slices.Clone(m.Selections)
	for i := range m.Selections {
		m.Selections[i].Price = jitteredPrice(m.Selections[i].Price, observedAt, maxJitter)
	}
	return m
}

func jitteredPrice(p canon.Price, observedAt time.Time, maxJitter float64) canon.Price {
	p.ObservedAt = observedAt
	if p.Form == canon.FormLineOnly || p.Raw <= 0 {
		return p
	}
	factor := 1 + (rand.Float64()*2-1)*maxJitter //nolint:gosec
	p.Raw = clampToForm(p.Raw*factor, p.Form)
	p.Prob = canon.ImpliedProb(p.Raw, p.Form)
	if p.Ask != nil {
		p.Ask = new(p.Raw)
	}
	if p.Bid != nil {
		p.Bid = new(min(clampToForm(*p.Bid*factor, p.Form), p.Raw))
	}
	return p
}

func clampToForm(raw float64, form canon.PriceForm) float64 {
	switch form {
	case canon.FormShare:
		return min(max(raw, 0.01), 0.99)
	case canon.FormCents, canon.FormNaira:
		return min(max(raw, 1), 99)
	case canon.FormDecimal:
		return max(raw, 1.01)
	default:
		return raw
	}
}
