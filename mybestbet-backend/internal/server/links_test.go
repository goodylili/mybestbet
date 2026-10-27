package server

import (
	"context"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/sources"
)

type stubLinker struct{}

func (stubLinker) EventURL(ref sources.EventRef) string {
	link := "https://book.test/" + ref.SourceEventID + "/" + sources.Slug(ref.Home, "-") + "-vs-" + sources.Slug(ref.Away, "-") + "/" + ref.LeagueName
	if ref.SourceLeagueID != "" {
		link += "?league=" + ref.SourceLeagueID
	}
	return link
}

func TestEventLinksURL(t *testing.T) {
	t.Parallel()
	links := newEventLinks(config.Config{Leagues: []config.League{{Code: "EPL", Name: "Premier League", Country: "England"}}})
	links.linkers["stub"] = stubLinker{}
	event := EventInfo{LeagueCode: "EPL", Home: Team{Name: "Chelsea"}, Away: Team{Name: "Manchester United"}}
	tests := []struct {
		name           string
		source         string
		sourceLeagueID string
		flipped        bool
		want           string
	}{
		{name: "book orientation", source: "stub", want: "https://book.test/42/chelsea-vs-manchester-united/Premier League"},
		{name: "flipped link swaps the teams", source: "stub", flipped: true, want: "https://book.test/42/manchester-united-vs-chelsea/Premier League"},
		{name: "stored source league reaches the linker", source: "stub", sourceLeagueID: "88637", want: "https://book.test/42/chelsea-vs-manchester-united/Premier League?league=88637"},
		{name: "unknown source has no link", source: "nope", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := links.url(tt.source, "42", tt.sourceLeagueID, tt.flipped, event); got != tt.want {
				t.Errorf("url() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEventLinksEncode(t *testing.T) {
	t.Parallel()
	links := map[string]string{"betking": "https://m.betking.com/en-ng/sports/prematch/42/a-vs-b"}
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "summary without links", value: EventSummary{EventInfo: EventInfo{ID: "e1"}}, want: ""},
		{name: "summary with links", value: EventSummary{EventInfo: EventInfo{ID: "e1", Links: links}}, want: links["betking"]},
		{name: "signal with links", value: SignalView{Event: EventInfo{ID: "e1", Links: links}}, want: links["betking"]},
		{name: "detail with links", value: EventDetail{Event: EventInfo{ID: "e1", Links: links}}, want: links["betking"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if tt.want == "" && strings.Contains(string(encoded), `"links"`) {
				t.Errorf("empty links encoded: %s", encoded)
			}
			if tt.want != "" && !strings.Contains(string(encoded), tt.want) {
				t.Errorf("link missing from %s", encoded)
			}
		})
	}
}

type marketOnlyLinker struct{}

func (marketOnlyLinker) EventURL(sources.EventRef) string { return "" }

func TestAttachEventLinksFallsBackToMainMarketDeepLink(t *testing.T) {
	pool, rdb := integrationDeps(t)
	f := seedContractFixture(t, pool, rdb)
	s := newTestServer(t, pool, rdb, f)
	s.links.linkers["marketonly"] = marketOnlyLinker{}
	s.links.linkers["stub"] = stubLinker{}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, source := range []string{"marketonly", "stub"} {
		_, err := pool.Exec(ctx, `INSERT INTO event_links (source, "sourceEventId", "eventId") VALUES ($1, 'game-1', $2)
			ON CONFLICT (source, "sourceEventId") DO UPDATE SET "eventId" = EXCLUDED."eventId"`, source, f.eventID)
		require.NoError(t, err)
	}
	prices := []struct{ key, source, code, link string }{
		{key: f.propKey, source: "marketonly", code: "P_ANYTIME", link: "https://book.test/market/scorer"},
		{key: oneXTwoKey(f.eventID), source: "marketonly", code: "1X2", link: "https://book.test/market/winner"},
		{key: oneXTwoKey(f.eventID), source: "stub", code: "1X2", link: "https://book.test/market/stub-winner"},
	}
	for _, p := range prices {
		_, err := pool.Exec(ctx, `INSERT INTO prices_current ("marketKey", source, side, "eventId", code, raw, form, prob, "deepLink", "observedAt")
			VALUES ($1, $2, 'home', $3, $4, 2, 'decimal', .5, $5, $6)
			ON CONFLICT ("marketKey", source, side, label) DO UPDATE SET "deepLink" = EXCLUDED."deepLink"`, p.key, p.source, f.eventID, p.code, p.link, now)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM event_links WHERE source IN ('marketonly', 'stub') AND "eventId" = $1`, f.eventID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM prices_current WHERE source IN ('marketonly', 'stub') AND "eventId" = $1`, f.eventID)
	})

	events := []EventInfo{{ID: f.eventID, LeagueCode: "EPL", Home: Team{Name: "Chelsea"}, Away: Team{Name: "Arsenal"}}}
	require.NoError(t, s.attachEventLinks(ctx, events))

	assert.Equal(t, "https://book.test/market/winner", events[0].Links["marketonly"])
	assert.True(t, strings.HasPrefix(events[0].Links["stub"], "https://book.test/game-1/"), events[0].Links["stub"])
}
