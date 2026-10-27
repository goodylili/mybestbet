package pricing

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const tolerance = 1e-9

func TestImpliedProbRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		raw      float64
		form     canon.PriceForm
		expected float64
	}{
		{name: "decimal odds", raw: 2.5, form: canon.FormDecimal, expected: 0.4},
		{name: "polymarket share", raw: 0.37, form: canon.FormShare, expected: 0.37},
		{name: "kalshi cents", raw: 42, form: canon.FormCents, expected: 0.42},
		{name: "bayse naira", raw: 61, form: canon.FormNaira, expected: 0.61},
		{name: "prizepicks line only", raw: 0, form: canon.FormLineOnly, expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prob := canon.ImpliedProb(tt.raw, tt.form)
			assert.InDelta(t, tt.expected, prob, tolerance)
			assert.InDelta(t, tt.raw, RawFromProb(prob, tt.form), tolerance)
		})
	}
}

func TestDevig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		implied  []float64
		method   DevigMethod
		expected []float64
	}{
		{name: "multiplicative two way", implied: []float64{0.55, 0.5}, method: DevigMultiplicative, expected: []float64{0.55 / 1.05, 0.5 / 1.05}},
		{name: "multiplicative three way", implied: []float64{0.5, 0.3, 0.25}, method: DevigMultiplicative, expected: []float64{0.5 / 1.05, 0.3 / 1.05, 0.25 / 1.05}},
		{name: "power already fair", implied: []float64{0.5, 0.3, 0.2}, method: DevigPower, expected: []float64{0.5, 0.3, 0.2}},
		{name: "power three way shades longshot", implied: []float64{0.6, 0.25, 0.2}, method: DevigPower, expected: powerReference([]float64{0.6, 0.25, 0.2})},
		{name: "power underround", implied: []float64{0.45, 0.45}, method: DevigPower, expected: []float64{0.5, 0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fair, ok := Devig(tt.implied, tt.method)
			require.True(t, ok)
			require.Len(t, fair, len(tt.expected))
			for i := range fair {
				assert.InDelta(t, tt.expected[i], fair[i], 1e-6)
			}
		})
	}
}

func powerReference(implied []float64) []float64 {
	lo, hi := 1.0, 3.0
	for range 200 {
		mid := (lo + hi) / 2
		if poweredSum(implied, mid) > 1 {
			lo = mid
		} else {
			hi = mid
		}
	}
	out := make([]float64, len(implied))
	for i, q := range implied {
		out[i] = math.Pow(q, lo)
	}
	return out
}

func TestDevigPowerFavouriteKeepsMoreThanMultiplicative(t *testing.T) {
	t.Parallel()
	implied := []float64{0.7, 0.2, 0.15}
	power, ok := Devig(implied, DevigPower)
	require.True(t, ok)
	mult, _ := Devig(implied, DevigMultiplicative)
	assert.Greater(t, power[0], mult[0])
	assert.Less(t, power[2], mult[2])
}

func TestDevigRejectsInvalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		implied []float64
	}{
		{name: "single outcome", implied: []float64{0.5}},
		{name: "zero price", implied: []float64{0.5, 0}},
		{name: "certain outcome", implied: []float64{1, 0.2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, ok := Devig(tt.implied, DevigPower)
			assert.False(t, ok)
		})
	}
}

func TestDevigSumsToOneProperty(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(7, 11))
	for _, method := range []DevigMethod{DevigPower, DevigMultiplicative} {
		for range 2000 {
			n := 2 + rng.IntN(12)
			implied := make([]float64, n)
			for i := range implied {
				implied[i] = 0.01 + rng.Float64()*0.9
			}
			fair, ok := Devig(implied, method)
			require.True(t, ok)
			var sum float64
			for _, p := range fair {
				require.GreaterOrEqual(t, p, 0.0)
				sum += p
			}
			require.InDelta(t, 1, sum, tolerance, "method %s implied %v", method, implied)
		}
	}
}

func TestMethodFor(t *testing.T) {
	t.Parallel()
	p := config.Pricing{DefaultDevig: "power", DevigMethod: map[canon.MarketCode]string{canon.CodeOU: "multiplicative"}}
	assert.Equal(t, DevigMultiplicative, MethodFor(p, canon.CodeOU))
	assert.Equal(t, DevigPower, MethodFor(p, canon.Code1X2))
	assert.Equal(t, DevigPower, MethodFor(config.Pricing{}, canon.CodeCS))
}

func TestWeightedMedian(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		values   []WeightedProb
		expected float64
	}{
		{name: "equal weights odd count", values: []WeightedProb{{Prob: 0.3, Weight: 1}, {Prob: 0.5, Weight: 1}, {Prob: 0.4, Weight: 1}}, expected: 0.4},
		{name: "equal weights even count averages middle", values: []WeightedProb{{Prob: 0.3, Weight: 1}, {Prob: 0.5, Weight: 1}, {Prob: 0.4, Weight: 1}, {Prob: 0.6, Weight: 1}}, expected: 0.45},
		{name: "heavy source dominates", values: []WeightedProb{{Prob: 0.3, Weight: 1}, {Prob: 0.5, Weight: 3}, {Prob: 0.4, Weight: 1}}, expected: 0.5},
		{name: "single value", values: []WeightedProb{{Prob: 0.42, Weight: 2}}, expected: 0.42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.expected, WeightedMedian(tt.values), tolerance)
		})
	}
}

func TestConsensusNeedsMinimumSources(t *testing.T) {
	t.Parallel()
	two := []WeightedProb{{Feed: "a", Prob: 0.4, Weight: 1}, {Feed: "b", Prob: 0.5, Weight: 1}}
	_, ok := Consensus(two, 3)
	assert.False(t, ok)
	fair, ok := Consensus(append(two, WeightedProb{Feed: "c", Prob: 0.45, Weight: 1}), 3)
	require.True(t, ok)
	assert.InDelta(t, 0.45, fair, tolerance)
}

func TestConsensusCollapsesSameFeed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		values   []WeightedProb
		expects  bool
		expected float64
	}{
		{
			name:    "three brands of one feed count as one",
			values:  []WeightedProb{{Feed: "onexbet", Prob: 0.6, Weight: 1}, {Feed: "onexbet", Prob: 0.6, Weight: 1}, {Feed: "onexbet", Prob: 0.6, Weight: 1}},
			expects: false,
		},
		{
			name:     "duplicate feed cannot outvote independent books",
			values:   []WeightedProb{{Feed: "onexbet", Prob: 0.6, Weight: 1}, {Feed: "onexbet", Prob: 0.6, Weight: 1}, {Feed: "onexbet", Prob: 0.6, Weight: 1}, {Feed: "betking", Prob: 0.4, Weight: 1}, {Feed: "betway", Prob: 0.45, Weight: 1}},
			expects:  true,
			expected: 0.45,
		},
		{
			name:     "brands of one feed vote with their mean and heaviest weight",
			values:   []WeightedProb{{Feed: "sporty", Prob: 0.5, Weight: 1}, {Feed: "sporty", Prob: 0.6, Weight: 3}, {Feed: "betking", Prob: 0.4, Weight: 1}, {Feed: "betway", Prob: 0.45, Weight: 1}},
			expects:  true,
			expected: 0.55,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fair, ok := Consensus(tt.values, 3)
			require.Equal(t, tt.expects, ok)
			assert.InDelta(t, tt.expected, fair, tolerance)
		})
	}
}

func TestExceedsMaxOverround(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		code     canon.MarketCode
		implied  []float64
		max      float64
		expected bool
	}{
		{name: "normal book margin", code: canon.Code1X2, implied: []float64{0.5, 0.3, 0.27}, max: 0.15},
		{name: "junk order book quoting both sides high", code: canon.CodeOU, implied: []float64{0.9, 0.98}, max: 0.15, expected: true},
		{name: "double chance sums to two", code: canon.CodeDC, implied: []float64{0.8, 0.72, 0.55}, max: 0.15},
		{name: "double chance over the scaled limit", code: canon.CodeDC, implied: []float64{0.95, 0.9, 0.6}, max: 0.15, expected: true},
		{name: "gate disabled", code: canon.CodeOU, implied: []float64{0.9, 0.98}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, ExceedsMaxOverround(tt.code, tt.implied, tt.max))
		})
	}
}

func TestDevigMarketDoubleChanceSumsToTwo(t *testing.T) {
	t.Parallel()
	fair, ok := DevigMarket(config.Pricing{DefaultDevig: "power"}, canon.CodeDC, []float64{0.8, 0.72, 0.55})
	require.True(t, ok)
	assert.InDelta(t, 2, fair[0]+fair[1]+fair[2], tolerance)
}

func TestSourceWeight(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Pricing: config.Pricing{DeepBookLiquidity: 1000, DeepBookWeight: 3},
		Sources: map[string]sources.Config{"betano": {Weight: 2}, "kalshi": {Weight: 1}},
	}
	tests := []struct {
		name     string
		source   string
		touch    canon.Price
		expected float64
	}{
		{name: "configured book weight", source: "betano", expected: 2},
		{name: "unknown book defaults to one", source: "bet9ja", expected: 1},
		{name: "deep prediction book", source: "kalshi", touch: canon.Price{Liquidity: new(1500.0)}, expected: 3},
		{name: "thin prediction book", source: "polymarket", touch: canon.Price{Liquidity: new(200.0)}, expected: 1},
		{name: "prediction book without depth", source: "polymarket", expected: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.expected, SourceWeight(cfg, tt.source, tt.touch), tolerance)
		})
	}
}

func TestParseBucket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		label    string
		expected Bucket
		ok       bool
	}{
		{label: "<24", expected: Bucket{Low: 0, High: 23}, ok: true},
		{label: "Under 24", expected: Bucket{Low: 0, High: 23}, ok: true},
		{label: "24-31", expected: Bucket{Low: 24, High: 31}, ok: true},
		{label: "24 - 31", expected: Bucket{Low: 24, High: 31}, ok: true},
		{label: "32+", expected: Bucket{Low: 32, High: openEnded}, ok: true},
		{label: "32 or more", expected: Bucket{Low: 32, High: openEnded}, ok: true},
		{label: "3 or fewer", expected: Bucket{Low: 0, High: 3}, ok: true},
		{label: "31-24", ok: false},
		{label: "lots", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseBucket(tt.label)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestLadder(t *testing.T) {
	t.Parallel()
	quotes := []BucketQuote{
		{Bucket: Bucket{Low: 32, High: openEnded}, Implied: 0.22, Fair: 0.2, Liquidity: new(80.0)},
		{Bucket: Bucket{Low: 0, High: 23}, Implied: 0.32, Fair: 0.3, Liquidity: new(500.0)},
		{Bucket: Bucket{Low: 24, High: 31}, Implied: 0.52, Fair: 0.5, Liquidity: new(300.0)},
	}
	rungs, ok := Ladder(quotes)
	require.True(t, ok)
	require.Len(t, rungs, 2)
	assert.InDelta(t, 23.5, rungs[0].Line, tolerance)
	assert.InDelta(t, 0.7, rungs[0].Over.Fair, tolerance)
	assert.InDelta(t, 0.3, rungs[0].Under.Fair, tolerance)
	assert.InDelta(t, 0.74, rungs[0].Over.Implied, tolerance)
	require.NotNil(t, rungs[0].Over.Liquidity)
	assert.InDelta(t, 80, *rungs[0].Over.Liquidity, tolerance)
	assert.InDelta(t, 500, *rungs[0].Under.Liquidity, tolerance)
	assert.InDelta(t, 31.5, rungs[1].Line, tolerance)
	assert.InDelta(t, 0.2, rungs[1].Over.Fair, tolerance)
	assert.InDelta(t, 0.8, rungs[1].Under.Fair, tolerance)
}

func TestLadderRejectsGaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		buckets []Bucket
	}{
		{name: "gap between buckets", buckets: []Bucket{{0, 20}, {24, 31}, {32, openEnded}}},
		{name: "no open top", buckets: []Bucket{{0, 23}, {24, 31}}},
		{name: "not from zero", buckets: []Bucket{{5, 23}, {24, openEnded}}},
		{name: "single bucket", buckets: []Bucket{{0, openEnded}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			quotes := make([]BucketQuote, len(tt.buckets))
			for i, b := range tt.buckets {
				quotes[i] = BucketQuote{Bucket: b, Implied: 0.3, Fair: 0.3}
			}
			_, ok := Ladder(quotes)
			assert.False(t, ok)
		})
	}
}

func TestDeriveFromThreeWay(t *testing.T) {
	t.Parallel()
	d, ok := DeriveFromThreeWay(ThreeWay{Home: 0.5, Draw: 0.3, Away: 0.2})
	require.True(t, ok)
	assert.InDelta(t, 0.8, d.HomeOrDraw, tolerance)
	assert.InDelta(t, 0.7, d.HomeOrAway, tolerance)
	assert.InDelta(t, 0.5, d.DrawOrAway, tolerance)
	assert.InDelta(t, 0.5/0.7, d.DrawNoBetHome, tolerance)
	assert.InDelta(t, 0.2/0.7, d.DrawNoBetAway, tolerance)
	assert.InDelta(t, 1, d.DrawNoBetHome+d.DrawNoBetAway, tolerance)
}

func TestDeriveDrawNoBetMatchesSyntheticHedge(t *testing.T) {
	t.Parallel()
	home, draw := 2.1, 3.4
	d, ok := DeriveFromThreeWay(ThreeWay{Home: 1 / home, Draw: 1 / draw, Away: 1 / 3.8})
	require.True(t, ok)
	homeStake := 1.0
	drawStake := homeStake / (draw - 1)
	syntheticDecimal := homeStake * home / (homeStake + drawStake)
	assert.InDelta(t, 1/syntheticDecimal, d.DrawNoBetHome, tolerance)
}

func TestWorstPushAwareReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     float64
		first    canon.Side
		dFirst   float64
		dSecond  float64
		expected float64
	}{
		{name: "half line behaves like plain two way", line: -0.5, first: canon.SideHome, dFirst: 2.1, dSecond: 2.1, expected: 105},
		{name: "whole line ignores full refund", line: 0, first: canon.SideHome, dFirst: 2.1, dSecond: 2.1, expected: 105},
		{name: "quarter line halves the edge on the split result", line: -0.25, first: canon.SideHome, dFirst: 2.1, dSecond: 2.1, expected: 102.5},
		{name: "totals quarter line", line: 2.25, first: canon.SideOver, dFirst: 2.1, dSecond: 2.1, expected: 102.5},
		{name: "losing pair", line: -0.5, first: canon.SideHome, dFirst: 1.9, dSecond: 1.9, expected: 95},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			offset, ok := PushOffset(tt.line, tt.first)
			require.True(t, ok)
			s := 1/tt.dFirst + 1/tt.dSecond
			first := TwoWayStake{Decimal: tt.dFirst, Stake: 100 / (tt.dFirst * s)}
			second := TwoWayStake{Decimal: tt.dSecond, Stake: 100 / (tt.dSecond * s)}
			assert.InDelta(t, tt.expected, WorstPushAwareReturn(offset, first, second), 1e-6)
		})
	}
}

func TestPushOffsetRejectsNonLeadingSides(t *testing.T) {
	t.Parallel()
	_, ok := PushOffset(1, canon.SideAway)
	assert.False(t, ok)
}

func TestPriceFromProb(t *testing.T) {
	t.Parallel()
	at := time.Unix(1700000000, 0)
	p := PriceFromProb(0.25, canon.FormDecimal, at)
	assert.InDelta(t, 4, p.Raw, tolerance)
	assert.Equal(t, at, p.ObservedAt)
}

func TestDevigMarketNeedsEveryOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		code    canon.MarketCode
		implied []float64
		ok      bool
	}{
		{name: "double chance missing 1X", code: canon.CodeDC, implied: []float64{0.94, 0.133}},
		{name: "full double chance", code: canon.CodeDC, implied: []float64{0.98, 0.94, 0.133}, ok: true},
		{name: "three-way missing the draw", code: canon.Code1X2, implied: []float64{0.5, 0.3}},
		{name: "two-way with an extra price", code: canon.CodeMoneyline, implied: []float64{0.5, 0.3, 0.3}},
		{name: "labeled market of any size", code: canon.CodeCS, implied: []float64{0.2, 0.3, 0.6}, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, ok := DevigMarket(config.Pricing{}, tt.code, tt.implied)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
