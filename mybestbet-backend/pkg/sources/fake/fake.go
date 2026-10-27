package fake

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const Name = "fake"

type Adapter struct {
	deps   sources.Deps
	events []canon.SourceEvent
}

func New(deps sources.Deps) sources.Adapter {
	kickoff := deps.Clock().Add(90 * time.Minute).Truncate(time.Minute)
	return &Adapter{deps: deps, events: []canon.SourceEvent{
		{SourceID: "fake-1", LeagueHint: "EPL", Home: "Arsenal", Away: "Chelsea", Kickoff: kickoff},
		{SourceID: "fake-2", LeagueHint: "LALIGA", Home: "Real Madrid", Away: "Barcelona", Kickoff: kickoff.Add(time.Hour)},
	}}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{Markets: []canon.MarketCode{canon.Code1X2, canon.CodeOU}, Leagues: []string{"EPL", "LALIGA"}}
}

func (a *Adapter) Discover(context.Context, []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	snap.Events = a.events
	return snap, nil
}

func (a *Adapter) Price(_ context.Context, eventIDs []string) (canon.Snapshot, error) {
	now := a.deps.Clock()
	snap := sources.NewSnapshot(Name, now)
	for _, id := range eventIDs {
		snap.Markets = append(snap.Markets, oneXTwo(id, now), overUnder(id, now))
	}
	return snap, nil
}

func jitteredOdds(base float64) float64 {
	return base * (0.97 + rand.Float64()*0.06) //nolint:gosec
}

func oneXTwo(eventID string, now time.Time) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  eventID,
		Code:           canon.Code1X2,
		SourceMarketID: fmt.Sprintf("%s-1x2", eventID),
		Rules:          canon.StandardRule,
		Selections: []canon.Selection{
			{Side: canon.SideHome, Price: canon.NewPrice(jitteredOdds(2.1), canon.FormDecimal, now)},
			{Side: canon.SideDraw, Price: canon.NewPrice(jitteredOdds(3.4), canon.FormDecimal, now)},
			{Side: canon.SideAway, Price: canon.NewPrice(jitteredOdds(3.6), canon.FormDecimal, now)},
		},
	}
}

func overUnder(eventID string, now time.Time) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  eventID,
		Code:           canon.CodeOU,
		Line:           canon.Line(2.5),
		SourceMarketID: fmt.Sprintf("%s-ou-2.5", eventID),
		Rules:          canon.StandardRule,
		Selections: []canon.Selection{
			{Side: canon.SideOver, Price: canon.NewPrice(jitteredOdds(1.9), canon.FormDecimal, now)},
			{Side: canon.SideUnder, Price: canon.NewPrice(jitteredOdds(1.95), canon.FormDecimal, now)},
		},
	}
}
