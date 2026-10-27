package canon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewShareBookPrice(t *testing.T) {
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name              string
		bid, ask, askSize float64
		form              PriceForm
		wantRaw, wantProb float64
		wantSuspended     bool
	}{
		{name: "tight share book", bid: 0.48, ask: 0.5, askSize: 100, form: FormShare, wantRaw: 0.5, wantProb: 0.5},
		{name: "cents conversion", bid: 0.36, ask: 0.37, askSize: 10, form: FormCents, wantRaw: 37, wantProb: 0.37},
		{name: "naira conversion", bid: 0.6, ask: 0.6125, askSize: 5, form: FormNaira, wantRaw: 61.25, wantProb: 0.6125},
		{name: "wide spread suspended", bid: 0.02, ask: 0.9, askSize: 100, form: FormShare, wantRaw: 0.9, wantProb: 0.9, wantSuspended: true},
		{name: "no ask size suspended", bid: 0.48, ask: 0.5, askSize: 0, form: FormShare, wantRaw: 0.5, wantProb: 0.5, wantSuspended: true},
		{name: "one sided suspended", bid: 0, ask: 0.5, askSize: 10, form: FormShare, wantRaw: 0.5, wantProb: 0.5, wantSuspended: true},
		{name: "empty book", form: FormShare, wantSuspended: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewShareBookPrice(tt.bid, tt.ask, tt.askSize, tt.form, at)
			assert.InDelta(t, tt.wantRaw, p.Raw, 1e-9)
			assert.InDelta(t, tt.wantProb, p.Prob, 1e-9)
			assert.Equal(t, tt.wantSuspended, p.Suspended)
			if tt.ask > 0 {
				require.NotNil(t, p.Liquidity)
				assert.InDelta(t, tt.ask*tt.askSize, *p.Liquidity, 1e-9)
			}
		})
	}
}
