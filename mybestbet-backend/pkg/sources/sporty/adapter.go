package sporty

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	json "github.com/goccy/go-json"
	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	SportyBet   = "sportybet"
	FootballCom = "footballcom"
)

const (
	listingMarketID      = "1"
	prematchProductID    = "3"
	requestTimeout       = 15 * time.Second
	minPollInterval      = 5 * time.Second
	concurrentEventFetch = 4
	emptyBodyRetries     = 2
	emptyBodyBackoff     = 400 * time.Millisecond
)

func init() {
	sources.Register(SportyBet, Factory(SportyBet))
	sources.Register(FootballCom, Factory(FootballCom))
}

type Adapter struct {
	name         string
	deps         sources.Deps
	competitions map[string]competition
	flaps        *flapGuard
}

func Factory(name string) sources.Factory {
	return func(deps sources.Deps) sources.Adapter {
		return &Adapter{name: name, deps: deps, competitions: competitionsWithOverrides(deps.Config.LeagueIDs), flaps: newFlapGuard()}
	}
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:     suppliedCodes(),
		Leagues:     slices.Sorted(maps.Keys(a.competitions)),
		Geo:         a.deps.Config.Geo,
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(a.name, a.deps.Clock())
	trees := map[string]sportTree{}
	for _, league := range leagues {
		c, ok := a.competitions[league]
		if !ok {
			continue
		}
		events, err := a.competitionEvents(ctx, c, trees)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("discover %s: %w", league, err))
		}
		for _, ev := range events {
			snap.Events = append(snap.Events, sourceEvent(ev, league))
		}
	}
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) competitionEvents(ctx context.Context, c competition, trees map[string]sportTree) ([]event, error) {
	tournamentIDs := []string{c.tournamentID}
	if c.tournamentID == "" {
		tree, err := a.sportTree(ctx, c.sportID, trees)
		if err != nil {
			return nil, err
		}
		tournamentIDs = tree.tournamentIDsMatching(c.sportID, c.byName)
	}
	if len(tournamentIDs) == 0 {
		return nil, nil
	}
	return a.tournamentEvents(ctx, c.sportID, tournamentIDs)
}

func (a *Adapter) sportTree(ctx context.Context, sportID string, trees map[string]sportTree) (sportTree, error) {
	if tree, ok := trees[sportID]; ok {
		return tree, nil
	}
	var resp envelope[sportTree]
	query := url.Values{"sportId": {sportID}, "productId": {prematchProductID}}
	if err := a.fetchJSON(ctx, http.MethodGet, "/factsCenter/popularAndSportList?"+query.Encode(), nil, &resp); err != nil {
		return sportTree{}, err
	}
	if err := resp.err(); err != nil {
		return sportTree{}, err
	}
	trees[sportID] = resp.Data
	return resp.Data, nil
}

func (t sportTree) tournamentIDsMatching(sportID string, name *tournamentName) []string {
	var ids []string
	for _, sport := range t.Sports {
		if sport.ID != sportID {
			continue
		}
		for _, category := range sport.Categories {
			for _, tournament := range category.Tournaments {
				if name == nil || name.matches(category.Name, tournament.Name) {
					ids = append(ids, tournament.ID)
				}
			}
		}
	}
	return ids
}

type eventsRequest struct {
	SportID       string     `json:"sportId"`
	MarketID      string     `json:"marketId"`
	TournamentIDs [][]string `json:"tournamentId"`
}

func (a *Adapter) tournamentEvents(ctx context.Context, sportID string, tournamentIDs []string) ([]event, error) {
	request := eventsRequest{SportID: sportID, MarketID: listingMarketID}
	for _, id := range tournamentIDs {
		request.TournamentIDs = append(request.TournamentIDs, []string{id})
	}
	body, err := json.Marshal([]eventsRequest{request})
	if err != nil {
		return nil, fmt.Errorf("encode events request: %w", err)
	}
	var resp envelope[[]tournament]
	if err := a.fetchJSON(ctx, http.MethodPost, "/factsCenter/pcEvents", body, &resp); err != nil {
		return nil, err
	}
	if err := resp.err(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var events []event
	for _, t := range resp.Data {
		for _, ev := range t.Events {
			if !seen[ev.EventID] {
				seen[ev.EventID] = true
				events = append(events, ev)
			}
		}
	}
	return events, nil
}

func sourceEvent(ev event, league string) canon.SourceEvent {
	home, away := ev.HomeTeamName, ev.AwayTeamName
	if sportOfSportradarID[ev.Sport.ID] == canon.SportTennis {
		home, away = givenNameFirst(home), givenNameFirst(away)
	}
	return canon.SourceEvent{
		SourceID:   ev.EventID,
		LeagueHint: league,
		Home:       home,
		Away:       away,
		Kickoff:    time.UnixMilli(ev.EstimateStartTime).UTC(),
	}
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	fetchedAt := a.deps.Clock()
	snap := sources.NewSnapshot(a.name, fetchedAt)
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
	a.flaps.suspendFlapping(snap.Markets, fetchedAt)
	return snap, sources.FailedEntirely(snap)
}

func (a *Adapter) eventMarkets(ctx context.Context, eventID string, fetchedAt time.Time) ([]canon.RawMarket, error) {
	ev, err := a.event(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("price event %s: %w", eventID, err)
	}
	return a.rawMarkets(ev, fetchedAt), nil
}

func (a *Adapter) event(ctx context.Context, eventID string) (event, error) {
	query := url.Values{"eventId": {eventID}, "productId": {prematchProductID}, "_t": {a.cacheBuster()}}
	var resp envelope[event]
	if err := a.fetchJSON(ctx, http.MethodGet, "/factsCenter/event?"+query.Encode(), nil, &resp); err != nil {
		return event{}, err
	}
	if err := resp.err(); err != nil {
		return event{}, err
	}
	return resp.Data, nil
}

func (a *Adapter) fetchJSON(ctx context.Context, method, pathAndQuery string, body []byte, out any) error {
	target := a.deps.Config.BaseURL + pathAndQuery
	for attempt := range emptyBodyRetries + 1 {
		if attempt > 0 {
			if err := sleepCtx(ctx, time.Duration(attempt)*emptyBodyBackoff); err != nil {
				return err
			}
		}
		raw, err := a.fetch(ctx, method, target, body)
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s: %w", target, err)
		}
		return nil
	}
	return fmt.Errorf("%s %s: empty body after %d attempts: %w", method, target, emptyBodyRetries+1, sources.ErrThrottled)
}

func (a *Adapter) fetch(ctx context.Context, method, target string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", target, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return a.deps.Do(req)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *Adapter) cacheBuster() string {
	return strconv.FormatInt(a.deps.Clock().UnixMilli(), 10)
}
