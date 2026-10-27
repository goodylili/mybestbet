package bet9ja

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"mybestbet/pkg/canon"
)

const playersSoccer = "Players Soccer"

type playerGroupKey struct {
	description string
	start       string
}

type playerGroups struct {
	mu      sync.RWMutex
	byEvent map[string]string
}

func (p *playerGroups) set(eventID, groupID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byEvent[eventID] = groupID
}

func (p *playerGroups) get(eventID string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	groupID, ok := p.byEvent[eventID]
	return groupID, ok
}

func (t sportsTree) playerGroupIndex() map[playerGroupKey]string {
	index := map[playerGroupKey]string{}
	for _, sport := range t.Sports {
		if sport.Name != playersSoccer {
			continue
		}
		for _, category := range sport.Categories {
			for id, group := range category.Groups {
				index[playerGroupKey{description: strings.TrimSpace(group.Name), start: group.Date}] = id
			}
		}
	}
	return index
}

type scorerMarket struct {
	key  string
	code canon.MarketCode
}

var scorerMarkets = []scorerMarket{
	{key: "GS_ANYSCORER_Yes", code: canon.PropAnytime},
	{key: "GS_1STSCORER_1STSCORER", code: canon.PropFirst},
	{key: "GS_LASTSCORER_LASTSCORER", code: canon.PropLast},
}

func playerMarkets(eventID string, players []event, observedAt time.Time) []canon.RawMarket {
	var out []canon.RawMarket
	for _, p := range players {
		player, ok := sourcePlayer(p)
		if !ok {
			continue
		}
		for _, m := range scorerMarkets {
			price := float64(p.Odds[m.key])
			if price <= 1 {
				continue
			}
			out = append(out, canon.RawMarket{
				EventSourceID:  eventID,
				Code:           m.code,
				Player:         player,
				Selections:     []canon.Selection{{Side: canon.SideYes, Price: canon.NewPrice(price, canon.FormDecimal, observedAt)}},
				Rules:          m.code.DefaultRule(),
				SourceMarketID: m.key + "@" + strconv.FormatInt(p.ID, 10),
			})
		}
	}
	return out
}

func sourcePlayer(p event) (*canon.SourcePlayer, bool) {
	desc := strings.TrimSpace(p.Description)
	var team string
	if rest, ok := strings.CutPrefix(desc, "("); ok {
		team, desc, _ = strings.Cut(rest, ")")
		desc = strings.TrimSpace(desc)
	}
	name := desc
	if last, first, ok := strings.Cut(desc, ","); ok {
		name = strings.TrimSpace(first) + " " + strings.TrimSpace(last)
	}
	if name == "" {
		return nil, false
	}
	return &canon.SourcePlayer{SourceID: name, Name: name, Team: strings.TrimSpace(team)}, true
}
