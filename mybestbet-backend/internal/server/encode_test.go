package server

import (
	stdjson "encoding/json"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestResponsesEncodeConcurrentlyLikeStdlib(t *testing.T) {
	t.Parallel()
	kickoff := time.Date(2026, 10, 27, 19, 0, 0, 0, time.UTC)
	items := make([]EventSummary, 300)
	for i := range items {
		items[i] = EventSummary{
			EventInfo: EventInfo{ID: "e", Sport: canon.SportFootball, LeagueCode: "EPL", Home: Team{ID: "h", Name: "Home"}, Away: Team{ID: "a", Name: "Away"}, Kickoff: kickoff, Status: "scheduled"},
			Sources:   []string{"betking"},
		}
		if i%2 == 0 {
			items[i].Links = map[string]string{"betking": "https://book.test/e"}
			items[i].Score = &canon.EventScore{Home: 1, Away: 0}
		}
	}
	bodies := []any{
		map[string]any{"items": items},
		[]SignalView{{Event: items[0].EventInfo}, {Event: items[1].EventInfo}},
		EventDetail{Event: items[0].EventInfo, Sources: []string{}, Markets: []MarketTab{}},
		PropBoard{
			Event:   items[0].EventInfo,
			Sources: []string{"betking"},
			Players: []PropPlayerInfo{{PropPlayer: PropPlayer{ID: "p", Name: "Player"}, MinutesTrend: []MinutesPoint{}}},
			Rows: []PropRow{{KeyString: "k", Selections: []PropSelection{{
				Side:  "over",
				Cells: map[string]PropCell{"betking": {Raw: 1.9}, "kalshi": {Raw: 0.52, Form: canon.FormShare}},
			}}}},
		},
	}
	var wg sync.WaitGroup
	for g := range 32 {
		wg.Go(func() {
			body := bodies[g%len(bodies)]
			got, err := json.Marshal(body)
			require.NoError(t, err)
			want, err := stdjson.Marshal(body)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
	wg.Wait()
}
