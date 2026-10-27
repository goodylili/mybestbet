package betpawa

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"time"

	json "github.com/goccy/go-json"
	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name                 = "betpawa"
	listPath             = "/events/lists/by-queries?q="
	categoryTreePath     = "/categories/list/"
	eventPath            = "/events/"
	listViewMarketType   = "3743"
	upcomingEvents       = "UPCOMING"
	noUpcomingEvents     = "0"
	pageSize             = 100
	maxPages             = 20
	concurrentEventFetch = 4
	requestTimeout       = 15 * time.Second
	minPollInterval      = 15 * time.Second
	homePosition         = 1
	awayPosition         = 2
)

func init() { sources.Register(Name, New) }

type Adapter struct {
	deps         sources.Deps
	competitions map[string]competitionRef
}

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{deps: deps, competitions: competitionsWithOverrides(deps.Config.LeagueIDs)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	codes := map[canon.MarketCode]bool{}
	for _, spec := range marketSpecs {
		codes[spec.code] = true
	}
	return sources.Capabilities{
		Markets:     slices.Sorted(maps.Keys(codes)),
		Leagues:     slices.Sorted(maps.Keys(a.competitions)),
		Geo:         "NG",
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	leaguesByCategory := a.requestedLeaguesByCategory(leagues)
	for _, cat := range slices.Sorted(maps.Keys(leaguesByCategory)) {
		leagueByCompetition, err := a.scheduledCompetitions(ctx, cat, leaguesByCategory[cat])
		if err != nil {
			snap.Errors = append(snap.Errors, err)
			continue
		}
		events, err := a.categoryEvents(ctx, cat, leagueByCompetition)
		if err != nil {
			snap.Errors = append(snap.Errors, err)
			continue
		}
		snap.Events = append(snap.Events, events...)
	}
	if len(snap.Events) == 0 && len(snap.Errors) > 0 {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) requestedLeaguesByCategory(leagues []string) map[categoryID][]string {
	byCategory := map[categoryID][]string{}
	for _, code := range leagues {
		if ref, ok := a.competitions[code]; ok {
			byCategory[ref.category] = append(byCategory[ref.category], code)
		}
	}
	return byCategory
}

func (a *Adapter) scheduledCompetitions(ctx context.Context, cat categoryID, leagues []string) (map[string]string, error) {
	var tree categoryTreeResponse
	if err := a.getJSON(ctx, categoryTreePath+url.PathEscape(string(cat)), &tree); err != nil {
		return nil, fmt.Errorf("betpawa category %s tree: %w", cat, err)
	}
	leagueByCompetition := map[string]string{}
	for _, t := range tree.WithRegions {
		for _, r := range t.Regions {
			for _, node := range r.Competitions {
				if node.EventCounts.Upcoming == noUpcomingEvents {
					continue
				}
				if code, ok := a.leagueOf(leagues, r.Region.Name, node.Competition); ok {
					leagueByCompetition[node.Competition.ID] = code
				}
			}
		}
	}
	return leagueByCompetition, nil
}

func (a *Adapter) leagueOf(leagues []string, region string, c competition) (string, bool) {
	for _, code := range leagues {
		if a.competitions[code].matches(region, c) {
			return code, true
		}
	}
	return "", false
}

func (a *Adapter) categoryEvents(ctx context.Context, cat categoryID, leagueByCompetition map[string]string) ([]canon.SourceEvent, error) {
	if len(leagueByCompetition) == 0 {
		return nil, nil
	}
	competitions := slices.Sorted(maps.Keys(leagueByCompetition))
	var found []canon.SourceEvent
	for page := range maxPages {
		events, err := a.upcomingEvents(ctx, cat, competitions, page*pageSize)
		if err != nil {
			return found, err
		}
		for _, ev := range events {
			if sourceEvent, ok := toSourceEvent(ev, leagueByCompetition); ok {
				found = append(found, sourceEvent)
			}
		}
		if len(events) < pageSize {
			break
		}
	}
	return found, nil
}

func (a *Adapter) upcomingEvents(ctx context.Context, cat categoryID, competitions []string, skip int) ([]event, error) {
	query, err := json.Marshal(listQuery{Queries: []eventQuery{{
		Query: eventFilter{
			EventType:  upcomingEvents,
			Categories: []categoryID{cat},
			Zones:      queryZones{Competitions: competitions},
			HasOdds:    true,
		},
		View: queryView{MarketTypes: []string{listViewMarketType}},
		Skip: skip,
		Take: pageSize,
	}}})
	if err != nil {
		return nil, fmt.Errorf("betpawa encode query: %w", err)
	}
	var resp listResponse
	if err := a.getJSON(ctx, listPath+url.QueryEscape(string(query)), &resp); err != nil {
		return nil, fmt.Errorf("betpawa list events: %w", err)
	}
	var events []event
	for _, page := range resp.Responses {
		events = append(events, page.Responses...)
	}
	return events, nil
}

func toSourceEvent(ev event, leagueByCompetition map[string]string) (canon.SourceEvent, bool) {
	league, ok := leagueByCompetition[ev.Competition.ID]
	home, away := participantName(ev.Participants, homePosition), participantName(ev.Participants, awayPosition)
	if !ok || home == "" || away == "" {
		return canon.SourceEvent{}, false
	}
	return canon.SourceEvent{SourceID: ev.ID, LeagueHint: league, Home: home, Away: away, Kickoff: ev.StartTime.UTC()}, true
}

func participantName(participants []participant, position int) string {
	for _, p := range participants {
		if p.Position == position {
			return p.Name
		}
	}
	return ""
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	perEvent := make([][]canon.RawMarket, len(eventIDs))
	perEventErr := make([]error, len(eventIDs))
	var group errgroup.Group
	group.SetLimit(concurrentEventFetch)
	for i, id := range eventIDs {
		group.Go(func() error {
			perEvent[i], perEventErr[i] = a.eventMarketsByID(ctx, id, snap.FetchedAt)
			return nil
		})
	}
	_ = group.Wait()
	priced := 0
	for i := range eventIDs {
		if perEventErr[i] != nil {
			snap.Errors = append(snap.Errors, perEventErr[i])
			continue
		}
		priced++
		snap.Markets = append(snap.Markets, perEvent[i]...)
	}
	if priced == 0 && len(snap.Errors) > 0 {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) eventMarketsByID(ctx context.Context, id string, fetchedAt time.Time) ([]canon.RawMarket, error) {
	ev, err := a.event(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.eventMarkets(ev, fetchedAt), nil
}

func (a *Adapter) event(ctx context.Context, id string) (event, error) {
	var ev event
	if err := a.getJSON(ctx, eventPath+url.PathEscape(id), &ev); err != nil {
		return event{}, fmt.Errorf("betpawa event %s: %w", id, err)
	}
	return ev, nil
}

func (a *Adapter) getJSON(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return a.deps.GetJSON(ctx, a.deps.Config.BaseURL+path, out)
}
