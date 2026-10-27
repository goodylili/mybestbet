package bayse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAMMPriceSuspendsUntradedAndUnquotedShares(t *testing.T) {
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		share         float64
		untraded      bool
		wantSuspended bool
	}{
		{name: "traded quote", share: 0.42, wantSuspended: false},
		{name: "untraded seed price", share: 0.509, untraded: true, wantSuspended: true},
		{name: "settled share", share: 1, wantSuspended: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ammPrice(tt.share, tt.untraded, at)
			assert.Equal(t, tt.wantSuspended, p.Suspended)
			assert.InDelta(t, tt.share*100, p.Raw, 1e-9)
		})
	}
}
