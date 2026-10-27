package pricing

import (
	"time"

	"mybestbet/pkg/canon"
)

const centsPerUnit = 100

func RawFromProb(prob float64, form canon.PriceForm) float64 {
	switch form {
	case canon.FormDecimal:
		if prob <= 0 {
			return 0
		}
		return 1 / prob
	case canon.FormShare:
		return prob
	case canon.FormCents, canon.FormNaira:
		return prob * centsPerUnit
	case canon.FormLineOnly:
		return 0
	}
	return 0
}

func PriceFromProb(prob float64, form canon.PriceForm, observedAt time.Time) canon.Price {
	return canon.Price{Raw: RawFromProb(prob, form), Form: form, Prob: prob, ObservedAt: observedAt}
}
