package resolve

import "mybestbet/pkg/canon"

type Selection struct {
	Line  *float64
	Side  canon.Side
	Label string
}

func Flip(code canon.MarketCode, s Selection) Selection {
	return Selection{Line: flippedLine(code, s.Line), Side: s.Side.Flipped(), Label: flippedLabel(code, s.Label)}
}
