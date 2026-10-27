package betway

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name             = "betway"
	regionsPath      = "/Feeds/RegionsAndLeagues/"
	catalogPath      = "/BetBook/Filtered/"
	eventMarketsPath = "/MarketGroupings/MarketGroupNamesAndMarketsForEvent"
	countryCode      = "NG"
	cultureCode      = "en-US"
	pageSize         = 50
	maxPages         = 20
	requestTimeout   = 15 * time.Second
	minPollInterval  = 15 * time.Second
)

func init() { sources.Register(Name, New) }

type Adapter struct {
	deps    sources.Deps
	leagues map[string]leagueRef
}

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{deps: deps, leagues: leaguesWithOverrides(deps.Config.LeagueIDs)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	codes := map[canon.MarketCode]bool{}
	for _, spec := range footballSpecs {
		codes[spec.code] = true
	}
	for _, spec := range headToHeadSpecs {
		codes[spec.code] = true
	}
	return sources.Capabilities{
		Markets:     slices.Sorted(maps.Keys(codes)),
		Leagues:     slices.Sorted(maps.Keys(a.leagues)),
		Geo:         countryCode,
		MinInterval: minPollInterval,
	}
}

type sportQuery struct {
	sport          sportID
	codeByLeague   map[string]string
	wholeSportCode string
}

func (q sportQuery) leagueCode(ev event) (string, bool) {
	if code, ok := q.codeByLeague[ev.RegionID+"/"+ev.LeagueID]; ok {
		return code, true
	}
	return q.wholeSportCode, q.wholeSportCode != ""
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	codesBySport := map[sportID][]string{}
	for _, code := range leagues {
		if ref, ok := a.leagues[code]; ok {
			codesBySport[ref.sport] = append(codesBySport[ref.sport], code)
		}
	}
	for _, sport := range slices.Sorted(maps.Keys(codesBySport)) {
		query, err := a.sportQuery(ctx, sport, codesBySport[sport])
		if err != nil {
			snap.Errors = append(snap.Errors, err)
			continue
		}
		events, err := a.sportEvents(ctx, query)
		if err != nil {
			snap.Errors = append(snap.Errors, err)
		}
		snap.Events = append(snap.Events, events...)
	}
	if len(snap.Events) == 0 && len(snap.Errors) > 0 {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) sportQuery(ctx context.Context, sport sportID, codes []string) (sportQuery, error) {
	listed, err := a.listedLeagues(ctx, sport)
	if err != nil {
		return sportQuery{}, err
	}
	query := sportQuery{sport: sport, codeByLeague: map[string]string{}}
	if len(listed) == 0 {
		return query, nil
	}
	for _, code := range codes {
		ref := a.leagues[code]
		switch {
		case ref.coversWholeSport():
			query.wholeSportCode = code
		case listed[ref.league]:
			query.codeByLeague[ref.league] = code
		}
	}
	return query, nil
}

func (a *Adapter) listedLeagues(ctx context.Context, sport sportID) (map[string]bool, error) {
	var tree regionsResponse
	if err := a.getJSON(ctx, regionsPath+url.PathEscape(string(sport)), url.Values{"countryCode": {countryCode}}, &tree); err != nil {
		return nil, fmt.Errorf("betway %s regions: %w", sport, err)
	}
	listed := map[string]bool{}
	for _, r := range tree.Regions {
		for _, l := range r.Leagues {
			listed[r.RegionID+"/"+l.LeagueID] = true
		}
	}
	return listed, nil
}

func (a *Adapter) sportEvents(ctx context.Context, query sportQuery) ([]canon.SourceEvent, error) {
	if query.wholeSportCode == "" && len(query.codeByLeague) == 0 {
		return nil, nil
	}
	var found []canon.SourceEvent
	for page := range maxPages {
		var catalog catalogResponse
		if err := a.getJSON(ctx, catalogPath, catalogQuery(query, page*pageSize), &catalog); err != nil {
			return found, fmt.Errorf("betway %s catalog: %w", query.sport, err)
		}
		for _, ev := range catalog.Events {
			code, ok := query.leagueCode(ev)
			if !ok || ev.IsLive {
				continue
			}
			found = append(found, canon.SourceEvent{
				SourceID:   strconv.FormatInt(ev.EventID, 10),
				LeagueHint: code,
				Home:       ev.HomeTeam,
				Away:       ev.AwayTeam,
				Kickoff:    time.Unix(ev.ExpectedStartEpoch, 0).UTC(),
			})
		}
		if catalog.IsFinalPage || len(catalog.Events) < pageSize {
			break
		}
	}
	return found, nil
}

func catalogQuery(query sportQuery, skip int) url.Values {
	q := url.Values{
		"countryCode": {countryCode},
		"cultureCode": {cultureCode},
		"sportId":     {string(query.sport)},
		"marketTypes": catalogMarketTypes[query.sport],
		"Skip":        {strconv.Itoa(skip)},
		"Take":        {strconv.Itoa(pageSize)},
	}
	if query.wholeSportCode != "" {
		return q
	}
	for i, key := range slices.Sorted(maps.Keys(query.codeByLeague)) {
		regionID, leagueID := splitLeagueKey(key)
		q.Set(fmt.Sprintf("RegionAndLeagueIds[%d].regionId", i), regionID)
		q.Set(fmt.Sprintf("RegionAndLeagueIds[%d].leagueId", i), leagueID)
	}
	return q
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	priced := 0
	for _, id := range eventIDs {
		resp, err := a.fetchEventMarkets(ctx, id)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("betway event %s: %w", id, err))
			continue
		}
		priced++
		snap.Markets = append(snap.Markets, a.eventMarkets(id, resp, snap.FetchedAt)...)
	}
	if priced == 0 && len(snap.Errors) > 0 {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) fetchEventMarkets(ctx context.Context, eventID string) (eventMarketsResponse, error) {
	var resp eventMarketsResponse
	q := url.Values{"eventId": {eventID}, "countryCode": {countryCode}, "cultureCode": {cultureCode}}
	err := a.getJSON(ctx, eventMarketsPath, q, &resp)
	return resp, err
}

func (a *Adapter) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return a.deps.GetJSON(ctx, a.deps.Config.BaseURL+path+"?"+query.Encode(), out)
}
