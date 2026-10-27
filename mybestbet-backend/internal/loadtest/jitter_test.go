package loadtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestJitteredPrice(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		price canon.Price
	}{
		{name: "decimal", price: canon.NewPrice(2.1, canon.FormDecimal, time.Time{})},
		{name: "decimal floor", price: canon.NewPrice(1.01, canon.FormDecimal, time.Time{})},
		{name: "share book", price: canon.NewBookPrice(new(0.98), new(0.99), new(500.0), canon.FormShare, time.Time{})},
		{name: "cents", price: canon.NewPrice(55, canon.FormCents, time.Time{})},
		{name: "line only", price: canon.NewPrice(2.5, canon.FormLineOnly, time.Time{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for range 200 {
				got := jitteredPrice(tt.price, at, 0.05)
				assert.Equal(t, at, got.ObservedAt)
				assert.InDelta(t, tt.price.Raw, got.Raw, tt.price.Raw*0.05+1e-9)
				assert.InDelta(t, canon.ImpliedProb(got.Raw, got.Form), got.Prob, 1e-12)
				assert.GreaterOrEqual(t, got.Prob, 0.0)
				assert.LessOrEqual(t, got.Prob, 1.0)
				if got.Ask != nil {
					assert.Equal(t, got.Raw, *got.Ask)
					assert.LessOrEqual(t, *got.Bid, *got.Ask)
				}
			}
		})
	}
}
