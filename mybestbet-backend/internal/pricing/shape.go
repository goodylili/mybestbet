package pricing

import "mybestbet/pkg/canon"

func FitsShape(code canon.MarketCode, side canon.Side) bool {
	spec, known := canon.SpecOf(code)
	if !known {
		return true
	}
	switch spec.Shape {
	case canon.ShapeThreeWay:
		return side == canon.SideHome || side == canon.SideDraw || side == canon.SideAway
	case canon.ShapeTwoWay, canon.ShapeHandicap:
		return side == canon.SideHome || side == canon.SideAway
	case canon.ShapeOverUnder:
		return side == canon.SideOver || side == canon.SideUnder
	case canon.ShapeYesNo:
		return side == canon.SideYes || side == canon.SideNo
	case canon.ShapeLabeled, canon.ShapeBuckets:
		return true
	}
	return true
}

const doubleChanceOutcomes = 3

func OutcomeCount(code canon.MarketCode) (int, bool) {
	if code == canon.CodeDC {
		return doubleChanceOutcomes, true
	}
	spec, _ := canon.SpecOf(code)
	switch spec.Shape {
	case canon.ShapeThreeWay:
		return 3, true
	case canon.ShapeTwoWay, canon.ShapeHandicap, canon.ShapeOverUnder, canon.ShapeYesNo:
		return 2, true
	case canon.ShapeLabeled, canon.ShapeBuckets:
		return 0, false
	}
	return 0, false
}

func SettlesAgainstLine(code canon.MarketCode) bool {
	spec, _ := canon.SpecOf(code)
	return spec.Shape == canon.ShapeHandicap || spec.Shape == canon.ShapeOverUnder || spec.Kind == canon.KindTeam
}
