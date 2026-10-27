package betking

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

const (
	Name                 = "betking"
	menuPath             = "/api/feeds/prematch/matches/custommenu/en/4/%d"
	tournamentPath       = "/api/feeds/prematch/en/4/%s/0/0"
	eventPath            = "/api/feeds/prematch/event/en/4/%s"
	requestTimeout       = 15 * time.Second
	concurrentEventFetch = 4
	minPollInterval      = 15 * time.Second
	homeTeamOrder        = 1
	awayTeamOrder        = 2
)

func init() { sources.Register(Name, New) }

type Adapter struct {
	deps        sources.Deps
	tournaments map[string]tournamentRef
}

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{deps: deps, tournaments: tournamentsWithOverrides(deps.Config.LeagueIDs)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     slices.Sorted(maps.Keys(marketCodes())),
		Leagues:     slices.Sorted(maps.Keys(a.tournaments)),
		Geo:         "NG",
		MinInterval: minPollInterval,
	}
}

func marketCodes() map[canon.MarketCode]bool {
	codes := map[canon.MarketCode]bool{}
	for _, spec := range marketSpecs {
		codes[spec.code] = true
	}
	for _, code := range playerStatCodes {
		codes[code] = true
	}
	return codes
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	leaguesBySport := a.requestedLeaguesBySport(leagues)
	for _, sport := range slices.Sorted(maps.Keys(leaguesBySport)) {
		scheduled, err := a.scheduledTournaments(ctx, sport, leaguesBySport[sport])
		if err != nil {
			snap.Errors = append(snap.Errors, err)
			continue
		}
		for _, tournamentID := range slices.Sorted(maps.Keys(scheduled)) {
			events, err := a.tournamentEvents(ctx, tournamentID, scheduled[tournamentID])
			if err != nil {
				snap.Errors = append(snap.Errors, err)
				continue
			}
			snap.Events = append(snap.Events, events...)
		}
	}
	return snap, failIfNothingFetched(len(snap.Events), snap.Errors)
}

func (a *Adapter) requestedLeaguesBySport(leagues []string) map[sportID][]string {
	bySport := map[sportID][]string{}
	for _, code := range leagues {
		if ref, ok := a.tournaments[code]; ok {
			bySport[ref.sport] = append(bySport[ref.sport], code)
		}
	}
	return bySport
}

func (a *Adapter) scheduledTournaments(ctx context.Context, sport sportID, leagues []string) (map[string]string, error) {
	var menu menuResponse
	if err := a.getJSON(ctx, fmt.Sprintf(menuPath, sport), &menu); err != nil {
		return nil, fmt.Errorf("betking menu for sport %d: %w", sport, err)
	}
	leagueByTournament := map[string]string{}
	for _, s := range menu.Sports {
		for _, category := range s.Category {
			for _, t := range category.Tournaments {
				if t.ItemEventCount == 0 {
					continue
				}
				if code, ok := a.leagueOf(leagues, category.ItemName, t); ok {
					leagueByTournament[strconv.FormatInt(t.ItemID, 10)] = code
				}
			}
		}
	}
	return leagueByTournament, nil
}

func (a *Adapter) leagueOf(leagues []string, category string, t menuTournament) (string, bool) {
	for _, code := range leagues {
		if a.tournaments[code].matches(category, t) {
			return code, true
		}
	}
	return "", false
}

func (a *Adapter) tournamentEvents(ctx context.Context, tournamentID, league string) ([]canon.SourceEvent, error) {
	var feed feedResponse
	if err := a.getJSON(ctx, fmt.Sprintf(tournamentPath, url.PathEscape(tournamentID)), &feed); err != nil {
		return nil, fmt.Errorf("betking tournament %s: %w", tournamentID, err)
	}
	var events []canon.SourceEvent
	for _, item := range mergeFragments(feed.AreaMatches) {
		home, away, ok := homeAndAway(item.Teams)
		if !ok {
			continue
		}
		events = append(events, canon.SourceEvent{
			SourceID:   strconv.FormatInt(item.ItemID, 10),
			LeagueHint: league,
			Home:       home,
			Away:       away,
			Kickoff:    item.ItemDate.UTC(),
		})
	}
	return events, nil
}

func homeAndAway(teams []team) (home, away string, ok bool) {
	for _, t := range teams {
		switch t.ItemOrder {
		case homeTeamOrder:
			home = t.Name
		case awayTeamOrder:
			away = t.Name
		}
	}
	return home, away, home != "" && away != ""
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	perEvent := make([][]canon.RawMarket, len(eventIDs))
	perEventErr := make([]error, len(eventIDs))
	var group errgroup.Group
	group.SetLimit(concurrentEventFetch)
	for i, id := range eventIDs {
		group.Go(func() error {
			perEvent[i], perEventErr[i] = a.eventMarkets(ctx, id, snap.FetchedAt)
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
	return snap, failIfNothingFetched(priced, snap.Errors)
}

func (a *Adapter) eventMarkets(ctx context.Context, id string, fetchedAt time.Time) ([]canon.RawMarket, error) {
	items, err := a.eventItems(ctx, id)
	if err != nil {
		return nil, err
	}
	var markets []canon.RawMarket
	for _, item := range items {
		markets = append(markets, a.itemMarkets(item, fetchedAt)...)
	}
	return markets, nil
}

func (a *Adapter) eventItems(ctx context.Context, id string) ([]matchItem, error) {
	var feed feedResponse
	if err := a.getJSON(ctx, fmt.Sprintf(eventPath, url.PathEscape(id)), &feed); err != nil {
		return nil, fmt.Errorf("betking event %s: %w", id, err)
	}
	return mergeFragments(feed.AreaMatches), nil
}

func (a *Adapter) getJSON(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return a.deps.GetJSON(ctx, a.deps.Config.BaseURL+path, out)
}

func failIfNothingFetched(fetched int, errs []error) error {
	if fetched == 0 && len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
