package bet365

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	sourceName        = "bet365"
	browserProfile    = "bet365"
	couponCapturePath = "matchmarketscontentapi"
	matchCapturePath  = "matchbettingcontentapi"
	pageWait          = 6 * time.Second
	fetchTimeout      = 60 * time.Second
	minInterval       = 20 * time.Second
)

var (
	ErrBrowserUnavailable = errors.New("bet365: browser unavailable")
	ErrNoOddsPayload      = errors.New("bet365: page produced no odds payload")
	ErrFixtureMissing     = errors.New("bet365: fixture missing from match page")
)

type Adapter struct {
	deps sources.Deps
}

func init() { sources.Register(sourceName, New) }

func New(deps sources.Deps) sources.Adapter { return &Adapter{deps: deps} }

func (a *Adapter) Name() string { return sourceName }

func (a *Adapter) Capabilities() sources.Capabilities {
	return sources.Capabilities{
		Markets:      suppliedMarkets,
		Leagues:      supportedLeagues(),
		NeedsBrowser: true,
		Geo:          a.deps.Config.Geo,
		MinInterval:  minInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(sourceName, a.deps.Clock())
	seen := map[string]bool{}
	for couponURL, coupon := range a.couponPlan(leagues) {
		fixtures, err := a.fetchFixtures(ctx, couponURL, couponCapturePath, false, "")
		if err != nil {
			return snap, fmt.Errorf("discover coupon %s: %w", couponURL, err)
		}
		for _, fx := range fixtures {
			event, ok := sourceEvent(fx, coupon, leagues)
			if ok && !seen[event.SourceID] {
				seen[event.SourceID] = true
				snap.Events = append(snap.Events, event)
			}
		}
	}
	return snap, nil
}

type coupon struct {
	league string
	class  sportClass
}

func (a *Adapter) couponPlan(leagues []string) map[string]coupon {
	plan := map[string]coupon{}
	for _, league := range leagues {
		class, supported := classBySport[sportByLeague[league]]
		if !supported {
			continue
		}
		if path := a.deps.Config.LeagueIDs[league]; path != "" {
			plan[a.pageURL(path)] = coupon{league: league, class: class}
			continue
		}
		plan[a.pageURL(fmt.Sprintf(couponPathFormat, class))] = coupon{class: class}
	}
	return plan
}

func sourceEvent(fx *fixture, c coupon, wanted []string) (canon.SourceEvent, bool) {
	t, ok := teamsFromFixtureName(fx.name)
	if !ok || fx.id == "" {
		return canon.SourceEvent{}, false
	}
	league := c.league
	if league == "" {
		league = leagueOfCompetition(fx.competition)
	}
	if league == "" || !slices.Contains(wanted, league) || classBySport[sportByLeague[league]] != c.class {
		return canon.SourceEvent{}, false
	}
	return canon.SourceEvent{
		SourceID:   eventSourceID(c.class, fx.id),
		LeagueHint: league,
		Home:       t.home,
		Away:       t.away,
		Kickoff:    parseKickoff(fx.startsAt),
	}, true
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(sourceName, a.deps.Clock())
	for _, eventID := range eventIDs {
		markets, err := a.priceEvent(ctx, eventID, snap.FetchedAt)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("price event %s: %w", eventID, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		snap.Markets = append(snap.Markets, markets...)
	}
	if len(eventIDs) > 0 && len(snap.Errors) == len(eventIDs) {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) priceEvent(ctx context.Context, eventID string, observedAt time.Time) ([]canon.RawMarket, error) {
	class, fixtureID := splitEventSourceID(eventID)
	fixtures, err := a.fetchFixtures(ctx, a.pageURL(fmt.Sprintf(matchPagePathFormat, class, fixtureID)), matchCapturePath, true, fixtureID)
	if err != nil {
		return nil, err
	}
	fx := mergeFixture(fixtures, fixtureID)
	if fx == nil {
		return nil, ErrFixtureMissing
	}
	t, ok := teamsFromFixtureName(fx.name)
	if !ok {
		return nil, fmt.Errorf("%w: fixture name %q", ErrFixtureMissing, fx.name)
	}
	specsByGroupName := marketsBySport[sportByClass[class]]
	var markets []canon.RawMarket
	for _, group := range fx.groups {
		spec, known := specsByGroupName[strings.ToLower(strings.TrimSpace(group.name))]
		if !known {
			a.deps.DropUnknownMarket(sourceName, group.id+":"+group.name)
			continue
		}
		converter := groupConverter{fixtureID: eventID, teams: t, group: group, spec: spec, observedAt: observedAt}
		markets = append(markets, converter.markets()...)
	}
	return markets, nil
}

func (a *Adapter) fetchFixtures(ctx context.Context, pageURL, capturePath string, captureWS bool, defaultFixtureID string) ([]*fixture, error) {
	if a.deps.Browser == nil {
		return nil, ErrBrowserUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	result, err := a.deps.Browser.Fetch(ctx, sources.BrowserRequest{
		Profile:     browserProfile,
		URL:         pageURL,
		CapturePath: []string{capturePath},
		CaptureWS:   captureWS,
		WaitMillis:  int(pageWait.Milliseconds()),
	})
	if err != nil {
		return nil, fmt.Errorf("browser fetch: %w", err)
	}
	var fixtures []*fixture
	for _, captured := range result.Captured {
		fixtures = append(fixtures, buildFixtures(parseSnapshotSections(captured.Body), defaultFixtureID)...)
	}
	if len(fixtures) == 0 {
		return nil, ErrNoOddsPayload
	}
	return fixtures, nil
}

func mergeFixture(fixtures []*fixture, id string) *fixture {
	var merged *fixture
	seenGroups := map[string]bool{}
	for _, fx := range fixtures {
		if fx.id != id {
			continue
		}
		if merged == nil {
			merged = &fixture{id: id}
		}
		if merged.name == "" {
			merged.name = fx.name
		}
		for _, g := range fx.groups {
			if !seenGroups[g.id] {
				seenGroups[g.id] = true
				merged.groups = append(merged.groups, g)
			}
		}
	}
	return merged
}

func (a *Adapter) pageURL(hashPath string) string {
	return strings.TrimRight(a.deps.Config.BaseURL, "/") + "/" + strings.TrimLeft(hashPath, "/")
}
