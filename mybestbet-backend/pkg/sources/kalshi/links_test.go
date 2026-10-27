package kalshi_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "kalshi", sources.Config{}, []sourcestest.LinkCase{
		{Name: "event ticker", Ref: sourcestest.EPLMatch("KXEPLGAME-26OCT31CFCMUN"), Want: "https://kalshi.com/markets/kxeplgame/chelsea-vs-manchester-united/kxeplgame-26oct31cfcmun"},
		{Name: "ticker without series", Ref: sources.EventRef{SourceEventID: "KXEPLGAME"}, Want: ""},
	})
}
