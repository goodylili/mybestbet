package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func oneXTwoMarket(eventID string, home float64, observedAt time.Time) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  eventID,
		Code:           canon.Code1X2,
		SourceMarketID: eventID + "-1x2",
		Rules:          canon.StandardRule,
		Selections: []canon.Selection{
			{Side: canon.SideHome, Price: canon.NewPrice(home, canon.FormDecimal, observedAt)},
			{Side: canon.SideDraw, Price: canon.NewPrice(3.4, canon.FormDecimal, observedAt)},
			{Side: canon.SideAway, Price: canon.NewPrice(3.6, canon.FormDecimal, observedAt)},
		},
	}
}

func TestMarketHashesSkipUnchangedMarkets(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	h := newMarketHashes()

	first := h.changed([]canon.RawMarket{oneXTwoMarket("e1", 2.1, t0)})
	require.Len(t, first, 1)
	assert.Len(t, h.changed([]canon.RawMarket{oneXTwoMarket("e1", 2.1, t0)}), 1, "uncommitted hashes must not suppress a retry")
	h.commit(first)

	assert.Empty(t, h.changed([]canon.RawMarket{oneXTwoMarket("e1", 2.1, t0.Add(5*time.Second))}), "only the observation time moved")
	assert.Len(t, h.changed([]canon.RawMarket{oneXTwoMarket("e1", 2.2, t0)}), 1, "price moved")

	suspended := oneXTwoMarket("e1", 2.1, t0)
	suspended.Selections[0].Price.Suspended = true
	assert.Len(t, h.changed([]canon.RawMarket{suspended}), 1, "suspension flipped")
	assert.Len(t, h.changed([]canon.RawMarket{oneXTwoMarket("e2", 2.1, t0)}), 1, "other event")
}

func TestMarketHashesForgetDroppedEvents(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	h := newMarketHashes()
	h.commit(h.changed([]canon.RawMarket{oneXTwoMarket("e1", 2.1, t0), oneXTwoMarket("e2", 2.1, t0)}))
	h.retainEvents([]string{"e2"})
	assert.Len(t, h.changed([]canon.RawMarket{oneXTwoMarket("e1", 2.1, t0)}), 1)
	assert.Empty(t, h.changed([]canon.RawMarket{oneXTwoMarket("e2", 2.1, t0)}))
}

func TestMarketHashesForgetMarketsMissingFromACompletePoll(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	h := newMarketHashes()
	kept, dropped := oneXTwoMarket("e1", 2.1, t0), oneXTwoMarket("e1", 2.1, t0)
	dropped.SourceMarketID = "e1-other"
	other := oneXTwoMarket("e2", 2.1, t0)
	h.commit(h.changed([]canon.RawMarket{kept, dropped, other}))

	h.forgetMissing([]canon.RawMarket{kept})

	assert.Empty(t, h.changed([]canon.RawMarket{kept, other}))
	assert.Len(t, h.changed([]canon.RawMarket{dropped}), 1)
}
