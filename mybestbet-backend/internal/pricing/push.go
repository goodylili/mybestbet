package pricing

import (
	"math"

	"mybestbet/pkg/canon"
)

const (
	resultTolerance    = 1e-9
	scenarioPadding    = 2
	quarterLineSpacing = 0.25
)

type TwoWayStake struct {
	Decimal float64
	Stake   float64
}

func PushOffset(line float64, first canon.Side) (float64, bool) {
	switch first {
	case canon.SideHome:
		return line, true
	case canon.SideOver:
		return -line, true
	case canon.SideDraw, canon.SideAway, canon.SideUnder, canon.SideYes, canon.SideNo:
		return 0, false
	}
	return 0, false
}

func WorstPushAwareReturn(firstOffset float64, first, second TwoWayStake) float64 {
	worst := math.Inf(1)
	lowest := int(math.Floor(-firstOffset)) - scenarioPadding
	highest := int(math.Ceil(-firstOffset)) + scenarioPadding
	for x := lowest; x <= highest; x++ {
		margin := float64(x) + firstOffset
		firstReturn, firstAllPush := legReturn(margin, first)
		secondReturn, secondAllPush := legReturn(-margin, second)
		if firstAllPush && secondAllPush {
			continue
		}
		worst = math.Min(worst, firstReturn+secondReturn)
	}
	return worst
}

func legReturn(margin float64, leg TwoWayStake) (float64, bool) {
	subMargins := []float64{margin}
	if isQuarterLine(margin) {
		subMargins = []float64{margin - quarterLineSpacing, margin + quarterLineSpacing}
	}
	weight := leg.Stake / float64(len(subMargins))
	var total float64
	allPush := true
	for _, m := range subMargins {
		switch {
		case m > resultTolerance:
			total += weight * leg.Decimal
			allPush = false
		case m < -resultTolerance:
			allPush = false
		default:
			total += weight
		}
	}
	return total, allPush
}

func isQuarterLine(v float64) bool {
	frac := math.Abs(v*2 - math.Round(v*2))
	return frac > resultTolerance
}
