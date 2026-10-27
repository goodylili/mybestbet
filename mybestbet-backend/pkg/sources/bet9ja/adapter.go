package bet9ja

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const Name = "bet9ja"

const (
	maxFallbackMarketGroups = 4
	requestTimeout          = 15 * time.Second
	minPollInterval         = 5 * time.Second
	concurrentEventFetch    = 4
)

func init() { sources.Register(Name, New) }

type Adapter struct {
	deps         sources.Deps
	competitions map[string]competition
	players      playerGroups
}

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{
		deps:         deps,
		competitions: competitionsWithOverrides(deps.Config.LeagueIDs),
		players:      playerGroups{byEvent: map[string]string{}},
	}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     suppliedCodes(),
		Leagues:     slices.Sorted(maps.Keys(a.competitions)),
		Geo:         a.deps.Config.Geo,
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	tree, treeErr := a.sportsTree(ctx)
	if treeErr != nil {
		snap.Errors = append(snap.Errors, fmt.Errorf("discover sports tree: %w", treeErr))
	}
	playersIndex := tree.playerGroupIndex()
	for _, league := range leagues {
		c, ok := a.competitions[league]
		if !ok {
			continue
		}
		groupIDs := []string{c.groupID}
		if c.byName != nil {
			if treeErr != nil {
				continue
			}
			groupIDs = tree.groupIDsNamed(*c.byName)
		}
		for _, groupID := range groupIDs {
			a.appendGroupEvents(ctx, &snap, league, groupID, playersIndex)
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) sportsTree(ctx context.Context) (sportsTree, error) {
	var resp response[sportsTree]
	if err := a.getJSON(ctx, "/PalimpsestAjax/GetSports?DISP=0", &resp); err != nil {
		return sportsTree{}, err
	}
	return resp.Data, resp.err()
}

func (a *Adapter) appendGroupEvents(
	ctx context.Context,
	snap *canon.Snapshot,
	league, groupID string,
	playersIndex map[playerGroupKey]string,
) {
	events, err := a.groupEvents(ctx, groupID)
	if err != nil {
		snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", league, err))
		return
	}
	for _, ev := range events {
		sourceEv, err := sourceEvent(ev, league)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", league, err))
			continue
		}
		snap.Events = append(snap.Events, sourceEv)
		if playersGroup, ok := playersIndex[playerGroupKey{description: ev.Description, start: ev.StartDate}]; ok {
			a.players.set(sourceEv.SourceID, playersGroup)
		}
	}
}

func (a *Adapter) groupEvents(ctx context.Context, groupID string) ([]event, error) {
	page, err := a.groupPage(ctx, "/PalimpsestAjax/GetEventsInGroup", url.Values{"GROUPID": {groupID}, "DISP": {"0"}})
	if err != nil || len(page.Events) > 0 {
		return page.Events, err
	}
	tried := 0
	for _, mg := range page.MarketGroups {
		if mg.ID == page.MainGroup || tried == maxFallbackMarketGroups {
			continue
		}
		tried++
		query := url.Values{"GROUPID": {groupID}, "DISP": {"0"}, "GROUPMARKETID": {mg.ID}, "matches": {"true"}}
		fallback, err := a.groupPage(ctx, "/PalimpsestAjax/GetEventsInGroupV2", query)
		if err != nil {
			return nil, err
		}
		if len(fallback.Events) > 0 {
			return fallback.Events, nil
		}
	}
	return nil, nil
}

func (a *Adapter) groupPage(ctx context.Context, path string, query url.Values) (groupPage, error) {
	var resp response[groupPage]
	if err := a.getJSON(ctx, path+"?"+query.Encode(), &resp); err != nil {
		return groupPage{}, err
	}
	return resp.Data, resp.err()
}

var errUnsplittableTeams = errors.New("description is not \"home - away\"")

func sourceEvent(ev event, league string) (canon.SourceEvent, error) {
	home, away, ok := ev.teams()
	if !ok {
		return canon.SourceEvent{}, fmt.Errorf("event %d: %w", ev.ID, errUnsplittableTeams)
	}
	kickoff, err := ev.kickoff()
	if err != nil {
		return canon.SourceEvent{}, err
	}
	return canon.SourceEvent{SourceID: strconv.FormatInt(ev.ID, 10), LeagueHint: league, Home: home, Away: away, Kickoff: kickoff}, nil
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	fetchedAt := a.deps.Clock()
	snap := sources.NewSnapshot(Name, fetchedAt)
	perEvent := make([][]canon.RawMarket, len(eventIDs))
	perEventErr := make([]error, len(eventIDs))
	var group errgroup.Group
	group.SetLimit(concurrentEventFetch)
	for i, eventID := range eventIDs {
		group.Go(func() error {
			perEvent[i], perEventErr[i] = a.eventMarkets(ctx, eventID, fetchedAt)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return snap, err
	}
	for i := range eventIDs {
		snap.Markets = append(snap.Markets, perEvent[i]...)
		if perEventErr[i] != nil {
			snap.Errors = append(snap.Errors, perEventErr[i])
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) eventMarkets(ctx context.Context, eventID string, fetchedAt time.Time) ([]canon.RawMarket, error) {
	ev, err := a.fetchEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("price event %s: %w", eventID, err)
	}
	markets := a.rawMarkets(ev, fetchedAt)
	playersGroup, ok := a.players.get(eventID)
	if !ok {
		return markets, nil
	}
	page, err := a.groupPage(ctx, "/PalimpsestAjax/GetEventsInGroup", url.Values{"GROUPID": {playersGroup}, "DISP": {"0"}})
	if err != nil {
		return markets, fmt.Errorf("price event %s players: %w", eventID, err)
	}
	return append(markets, playerMarkets(eventID, page.Events, fetchedAt)...), nil
}

func (a *Adapter) fetchEvent(ctx context.Context, eventID string) (event, error) {
	var resp response[event]
	if err := a.getJSON(ctx, "/PalimpsestAjax/GetEvent?"+url.Values{"EVENTID": {eventID}}.Encode(), &resp); err != nil {
		return event{}, err
	}
	return resp.Data, resp.err()
}

func (a *Adapter) getJSON(ctx context.Context, pathAndQuery string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return a.deps.GetJSON(ctx, a.deps.Config.BaseURL+pathAndQuery, out)
}
