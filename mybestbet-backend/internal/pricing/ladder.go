package pricing

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

type Bucket struct {
	Low  int
	High int
}

const openEnded = math.MaxInt

func (b Bucket) IsOpenEnded() bool { return b.High == openEnded }

func ParseBucket(label string) (Bucket, bool) {
	s := strings.ToLower(strings.TrimSpace(label))
	for _, parse := range []func(string) (Bucket, bool){parseRange, parseAtLeast, parseBelow, parseAtMost} {
		if b, ok := parse(s); ok && b.Low >= 0 && b.High >= b.Low {
			return b, true
		}
	}
	return Bucket{}, false
}

func parseRange(s string) (Bucket, bool) {
	lowText, highText, found := strings.Cut(s, "-")
	if !found {
		return Bucket{}, false
	}
	low, errLow := strconv.Atoi(strings.TrimSpace(lowText))
	high, errHigh := strconv.Atoi(strings.TrimSpace(highText))
	return Bucket{Low: low, High: high}, errLow == nil && errHigh == nil
}

func parseAtLeast(s string) (Bucket, bool) {
	for _, form := range []struct{ prefix, suffix string }{{"", "+"}, {"", " or more"}, {">=", ""}, {"at least ", ""}} {
		if n, ok := cutNumber(s, form.prefix, form.suffix); ok {
			return Bucket{Low: n, High: openEnded}, true
		}
	}
	return Bucket{}, false
}

func parseBelow(s string) (Bucket, bool) {
	for _, prefix := range []string{"<", "under ", "below ", "less than "} {
		if n, ok := cutNumber(s, prefix, ""); ok && n > 0 {
			return Bucket{Low: 0, High: n - 1}, true
		}
	}
	return Bucket{}, false
}

func parseAtMost(s string) (Bucket, bool) {
	for _, form := range []struct{ prefix, suffix string }{{"<=", ""}, {"", " or fewer"}, {"", " or less"}, {"at most ", ""}} {
		if n, ok := cutNumber(s, form.prefix, form.suffix); ok {
			return Bucket{Low: 0, High: n}, true
		}
	}
	return Bucket{}, false
}

func cutNumber(s, prefix, suffix string) (int, bool) {
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, suffix) || len(s) <= len(prefix)+len(suffix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(s[len(prefix) : len(s)-len(suffix)]))
	return n, err == nil
}

type BucketQuote struct {
	Bucket    Bucket
	Implied   float64
	Fair      float64
	Liquidity *float64
}

type LadderSide struct {
	Implied   float64
	Fair      float64
	Liquidity *float64
}

type LadderRung struct {
	Line  float64
	Over  LadderSide
	Under LadderSide
}

func Ladder(quotes []BucketQuote) ([]LadderRung, bool) {
	sorted := slices.Clone(quotes)
	slices.SortFunc(sorted, func(a, b BucketQuote) int { return a.Bucket.Low - b.Bucket.Low })
	if !bucketsPartitionAllCounts(sorted) {
		return nil, false
	}
	rungs := make([]LadderRung, 0, len(sorted)-1)
	for edge := 1; edge < len(sorted); edge++ {
		var rung LadderRung
		rung.Line = float64(sorted[edge].Bucket.Low) - 0.5
		for i, q := range sorted {
			if i >= edge {
				rung.Over.add(q)
			} else {
				rung.Under.add(q)
			}
		}
		rungs = append(rungs, rung)
	}
	return rungs, true
}

func bucketsPartitionAllCounts(sorted []BucketQuote) bool {
	if len(sorted) < 2 || sorted[0].Bucket.Low != 0 || !sorted[len(sorted)-1].Bucket.IsOpenEnded() {
		return false
	}
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].Bucket.IsOpenEnded() || sorted[i-1].Bucket.High+1 != sorted[i].Bucket.Low {
			return false
		}
	}
	return true
}

func (s *LadderSide) add(q BucketQuote) {
	s.Implied += q.Implied
	s.Fair += q.Fair
	if q.Liquidity != nil && (s.Liquidity == nil || *q.Liquidity < *s.Liquidity) {
		s.Liquidity = new(*q.Liquidity)
	}
}
