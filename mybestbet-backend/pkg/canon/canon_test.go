package canon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarketKeyRoundTrip(t *testing.T) {
	keys := []MarketKey{
		{EventID: "e1", Code: Code1X2},
		{EventID: "e1", Code: CodeOU, Line: Line(2.5)},
		{EventID: "e1", Code: PropShots, Line: Line(1.5), PlayerID: "p1"},
		{EventID: "e1", Code: Code1H1X2, Period: PeriodFirstHalf},
		{EventID: "e1", Code: TeamPasses, Line: Line(435.5), TeamID: "t1"},
	}
	for _, k := range keys {
		t.Run(k.String(), func(t *testing.T) {
			parsed, err := ParseMarketKey(k.String())
			require.NoError(t, err)
			require.Equal(t, k, parsed)
		})
	}
}

func TestParseMarketKeyRejectsWrongArity(t *testing.T) {
	_, err := ParseMarketKey("e1|1X2")
	require.Error(t, err)
}

func TestImpliedProb(t *testing.T) {
	tests := []struct {
		raw  float64
		form PriceForm
		want float64
	}{
		{2, FormDecimal, 0.5},
		{1, FormDecimal, 0},
		{0.42, FormShare, 0.42},
		{37, FormCents, 0.37},
		{55, FormNaira, 0.55},
		{2.5, FormLineOnly, 0},
	}
	for _, tt := range tests {
		require.InDelta(t, tt.want, ImpliedProb(tt.raw, tt.form), 1e-12)
	}
}
