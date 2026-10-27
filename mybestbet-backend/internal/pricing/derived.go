package pricing

type ThreeWay struct {
	Home, Draw, Away float64
}

type DerivedThreeWay struct {
	HomeOrDraw, HomeOrAway, DrawOrAway float64
	DrawNoBetHome, DrawNoBetAway       float64
}

const (
	LabelHomeOrDraw = "1X"
	LabelHomeOrAway = "12"
	LabelDrawOrAway = "X2"
)

func DeriveFromThreeWay(p ThreeWay) (DerivedThreeWay, bool) {
	if p.Home <= 0 || p.Draw <= 0 || p.Away <= 0 || p.Draw >= 1 {
		return DerivedThreeWay{}, false
	}
	return DerivedThreeWay{
		HomeOrDraw:    p.Home + p.Draw,
		HomeOrAway:    p.Home + p.Away,
		DrawOrAway:    p.Draw + p.Away,
		DrawNoBetHome: p.Home / (1 - p.Draw),
		DrawNoBetAway: p.Away / (1 - p.Draw),
	}, true
}
