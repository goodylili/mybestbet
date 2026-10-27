package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestWritePricesSkipsUnchangedRowsUntilRefreshDue(t *testing.T) {
	pool, _ := integrationDeps(t)
	ctx := context.Background()
	kickoff := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
	eventID := createTestEvent(t, pool, kickoff)
	s := &store{pool: pool}
	key := canon.MarketKey{EventID: eventID, Code: canon.MarketCode("1X2")}
	row := func(raw float64, observedAt time.Time) priceRow {
		return priceRow{marketKey: key.String(), eventID: eventID, code: key.Code, cell: canon.BoardCell{
			MarketKey: key.String(), Source: "bet9ja", Side: canon.SideHome,
			Price: canon.Price{Raw: raw, Form: canon.FormDecimal, Prob: 1 / raw, ObservedAt: observedAt},
		}}
	}
	stored := func(table string) (float64, time.Time) {
		var raw float64
		var observedAt time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT raw, "observedAt" FROM `+table+` WHERE "eventId" = $1`, eventID).Scan(&raw, &observedAt))
		return raw, observedAt.UTC()
	}
	first := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, s.writePrices(ctx, []priceRow{row(2.1, first)}))
	require.NoError(t, s.writePrices(ctx, []priceRow{row(2.1, first.Add(time.Minute))}))
	for _, table := range []string{"prices_current", "closing_prices"} {
		raw, observedAt := stored(table)
		assert.InDelta(t, 2.1, raw, 1e-9, table)
		assert.Equal(t, first, observedAt, table)
	}

	require.NoError(t, s.writePrices(ctx, []priceRow{row(2.2, first.Add(2*time.Minute))}))
	for _, table := range []string{"prices_current", "closing_prices"} {
		raw, observedAt := stored(table)
		assert.InDelta(t, 2.2, raw, 1e-9, table)
		assert.Equal(t, first.Add(2*time.Minute), observedAt, table)
	}

	require.NoError(t, s.writePrices(ctx, []priceRow{row(2.2, first.Add(13*time.Minute))}))
	for _, table := range []string{"prices_current", "closing_prices"} {
		_, observedAt := stored(table)
		assert.Equal(t, first.Add(13*time.Minute), observedAt, table)
	}
}

func TestWritePricesStoresAndRestoresSourceMarketKey(t *testing.T) {
	pool, _ := integrationDeps(t)
	ctx := context.Background()
	eventID := createTestEvent(t, pool, time.Now().Add(48*time.Hour).UTC().Truncate(time.Microsecond))
	s := &store{pool: pool}
	key := canon.MarketKey{EventID: eventID, Code: canon.Code1X2}
	first := time.Now().UTC().Truncate(time.Microsecond)
	row := func(sourceMarketKey string, observedAt time.Time) priceRow {
		return priceRow{marketKey: key.String(), eventID: eventID, code: key.Code, sourceMarketKey: sourceMarketKey, cell: canon.BoardCell{
			MarketKey: key.String(), Source: "bet9ja", Side: canon.SideHome,
			Price: canon.Price{Raw: 2.1, Form: canon.FormDecimal, Prob: 1 / 2.1, ObservedAt: observedAt},
		}}
	}
	stored := func() (string, time.Time) {
		var sourceMarketKey string
		var observedAt time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT "sourceMarketKey", "observedAt" FROM prices_current WHERE "eventId" = $1`, eventID).Scan(&sourceMarketKey, &observedAt))
		return sourceMarketKey, observedAt.UTC()
	}

	require.NoError(t, s.writePrices(ctx, []priceRow{row("1x2", first)}))
	require.NoError(t, s.writePrices(ctx, []priceRow{row("1x2", first.Add(time.Minute))}))
	sourceMarketKey, observedAt := stored()
	assert.Equal(t, "1x2", sourceMarketKey)
	assert.Equal(t, first, observedAt)

	require.NoError(t, s.writePrices(ctx, []priceRow{row("ft-1x2", first.Add(2*time.Minute))}))
	sourceMarketKey, observedAt = stored()
	assert.Equal(t, "ft-1x2", sourceMarketKey)
	assert.Equal(t, first.Add(2*time.Minute), observedAt)

	var restored []canon.Delta
	require.NoError(t, s.loadCurrentPrices(ctx, time.Now(), func(d canon.Delta) error {
		if d.Market.Key.EventID == eventID {
			restored = append(restored, d)
		}
		return nil
	}))
	require.Len(t, restored, 1)
	assert.Equal(t, "ft-1x2", restored[0].Market.SourceKey)
}
