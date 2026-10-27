package loadtest

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

const MaxInternalP95 = 300 * time.Millisecond

type Report struct {
	Path     string
	Sent     int
	Observed int
	Late     int
	Dropped  int
	P50      time.Duration
	P95      time.Duration
	P99      time.Duration
	Max      time.Duration
}

func newReport(path string, sent, late, unseen int, latencies []time.Duration) Report {
	sorted := slices.Sorted(slices.Values(latencies))
	r := Report{Path: path, Sent: sent, Observed: len(sorted) - late, Late: late, Dropped: late + unseen}
	if len(sorted) == 0 {
		return r
	}
	r.P50, r.P95, r.P99 = percentile(sorted, 0.50), percentile(sorted, 0.95), percentile(sorted, 0.99)
	r.Max = sorted[len(sorted)-1]
	return r
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	rank := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}

func (r Report) String() string {
	return fmt.Sprintf("%-13s sent=%d observed=%d dropped=%d (late=%d) p50=%s p95=%s p99=%s max=%s",
		r.Path, r.Sent, r.Observed, r.Dropped, r.Late, r.P50.Round(time.Microsecond), r.P95.Round(time.Microsecond), r.P99.Round(time.Microsecond), r.Max.Round(time.Microsecond))
}

func (r Report) breach() error {
	var breaches []error
	if r.Sent == 0 {
		breaches = append(breaches, fmt.Errorf("%s: no deltas sent", r.Path))
	}
	if r.Dropped > 0 {
		breaches = append(breaches, fmt.Errorf("%s: %d of %d deltas dropped", r.Path, r.Dropped, r.Sent))
	}
	if r.P95 > MaxInternalP95 {
		breaches = append(breaches, fmt.Errorf("%s: p95 %s over %s", r.Path, r.P95.Round(time.Millisecond), MaxInternalP95))
	}
	return errors.Join(breaches...)
}

func Verdict(reports []Report) error {
	breaches := make([]error, 0, len(reports))
	for _, r := range reports {
		breaches = append(breaches, r.breach())
	}
	return errors.Join(breaches...)
}
