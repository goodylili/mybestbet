package pricing

import (
	"math"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

type DevigMethod string

const (
	DevigPower          DevigMethod = "power"
	DevigMultiplicative DevigMethod = "multiplicative"
)

const (
	powerExponentMin      = 0.5
	powerExponentMax      = 3.0
	powerBisectionSteps   = 64
	powerSumTolerance     = 1e-12
	minOutcomesToDevig    = 2
	maxValidImpliedChance = 1.0
)

func MethodFor(p config.Pricing, code canon.MarketCode) DevigMethod {
	if m, ok := p.DevigMethod[code]; ok && m != "" {
		return DevigMethod(m)
	}
	if p.DefaultDevig != "" {
		return DevigMethod(p.DefaultDevig)
	}
	return DevigPower
}

const doubleChanceProbSum = 2

func fairProbSum(code canon.MarketCode) float64 {
	if code == canon.CodeDC {
		return doubleChanceProbSum
	}
	return 1
}

func DevigMarket(p config.Pricing, code canon.MarketCode, implied []float64) ([]float64, bool) {
	if outcomes, known := OutcomeCount(code); known && len(implied) != outcomes {
		return nil, false
	}
	total := fairProbSum(code)
	if total == 1 {
		return Devig(implied, MethodFor(p, code))
	}
	fair, ok := Devig(implied, DevigMultiplicative)
	for i := range fair {
		fair[i] *= total
	}
	return fair, ok
}

func ExceedsMaxOverround(code canon.MarketCode, implied []float64, maxOverround float64) bool {
	if maxOverround <= 0 {
		return false
	}
	var sum float64
	for _, q := range implied {
		sum += q
	}
	return sum > fairProbSum(code)*(1+maxOverround)
}

func Devig(implied []float64, method DevigMethod) ([]float64, bool) {
	if !isDevigable(implied) {
		return nil, false
	}
	if method == DevigPower {
		if fair, ok := devigPower(implied); ok {
			return fair, true
		}
	}
	return normalize(implied), true
}

func isDevigable(implied []float64) bool {
	if len(implied) < minOutcomesToDevig {
		return false
	}
	for _, q := range implied {
		if q <= 0 || q >= maxValidImpliedChance || math.IsNaN(q) {
			return false
		}
	}
	return true
}

func devigPower(implied []float64) ([]float64, bool) {
	lo, hi := powerExponentMin, powerExponentMax
	if poweredSum(implied, lo) < 1 || poweredSum(implied, hi) > 1 {
		return nil, false
	}
	for range powerBisectionSteps {
		mid := (lo + hi) / 2
		excess := poweredSum(implied, mid) - 1
		if math.Abs(excess) < powerSumTolerance {
			lo, hi = mid, mid
			break
		}
		if excess > 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	k := (lo + hi) / 2
	powered := make([]float64, len(implied))
	for i, q := range implied {
		powered[i] = math.Pow(q, k)
	}
	return normalize(powered), true
}

func poweredSum(implied []float64, k float64) float64 {
	var sum float64
	for _, q := range implied {
		sum += math.Pow(q, k)
	}
	return sum
}

func normalize(values []float64) []float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = v / sum
	}
	return out
}
