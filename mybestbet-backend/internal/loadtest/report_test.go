package loadtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func millis(values ...int) []time.Duration {
	out := make([]time.Duration, len(values))
	for i, v := range values {
		out[i] = time.Duration(v) * time.Millisecond
	}
	return out
}

func TestNewReport_Percentiles(t *testing.T) {
	t.Parallel()
	latencies := millis(100, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19)
	r := newReport(PathRedis, 22, 1, 1, latencies)
	assert.Equal(t, Report{Path: PathRedis, Sent: 22, Observed: 19, Late: 1, Dropped: 2,
		P50: 10 * time.Millisecond, P95: 19 * time.Millisecond, P99: 100 * time.Millisecond, Max: 100 * time.Millisecond}, r)
}

func TestNewReport_NoLatencies(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Report{Path: PathSSE, Sent: 3, Dropped: 3}, newReport(PathSSE, 3, 0, 3, nil))
}

func TestVerdict(t *testing.T) {
	t.Parallel()
	healthy := Report{Path: PathRedis, Sent: 10, Observed: 10, P95: MaxInternalP95}
	tests := []struct {
		name          string
		reports       []Report
		expectedFails []string
	}{
		{name: "p95 at threshold and no drops passes", reports: []Report{healthy}},
		{name: "one dropped delta fails", reports: []Report{{Path: PathRedis, Sent: 10, Observed: 9, Dropped: 1}}, expectedFails: []string{"1 of 10 deltas dropped"}},
		{name: "p95 over threshold fails", reports: []Report{{Path: PathSSE, Sent: 10, Observed: 10, P95: MaxInternalP95 + time.Millisecond}}, expectedFails: []string{"sse: p95"}},
		{name: "nothing sent fails", reports: []Report{{Path: PathRedis}}, expectedFails: []string{"no deltas sent"}},
		{name: "any failing path fails the run", reports: []Report{healthy, {Path: PathSSE, Sent: 4, Observed: 2, Dropped: 2}}, expectedFails: []string{"sse: 2 of 4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Verdict(tt.reports)
			if len(tt.expectedFails) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, fragment := range tt.expectedFails {
				assert.Contains(t, err.Error(), fragment)
			}
		})
	}
}
