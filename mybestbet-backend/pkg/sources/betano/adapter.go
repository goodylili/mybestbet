package betano

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	Name            = "betano"
	sportPath       = "/api/sport/%s/"
	matchOddsPath   = "/api/match-odds/%s/%s/"
	fieldsQuery     = "?req=s,stnf,c,mb,mbl"
	requestTimeout  = 15 * time.Second
	minPollInterval = 15 * time.Second
)

var errUnknownSlug = errors.New("betano: event slug unknown, discover the event first")

func init() { sources.Register(Name, New) }

type eventMeta struct {
	slug  string
	sport sportSlug
}

type Adapter struct {
	deps        sources.Deps
	leagueIDs   map[string]string
	metaMu      sync.RWMutex
	metaByEvent map[string]eventMeta
}

func New(deps sources.Deps) sources.Adapter {
	return &Adapter{deps: deps, leagueIDs: deps.Config.LeagueIDs, metaByEvent: map[string]eventMeta{}}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Capabilities() sources.Capabilities {
	codes := map[canon.MarketCode]bool{}
	for _, spec := range marketSpecs {
		if spec.selections != scorerTable {
			codes[spec.code] = true
		}
	}
	for _, spec := range headToHeadSpecs {
		codes[spec.code] = true
	}
	for _, code := range scorerColumnCodes {
		codes[code] = true
	}
	return sources.Capabilities{
		Markets:     slices.Sorted(maps.Keys(codes)),
		Leagues:     slices.Sorted(maps.Keys(defaultLeagueNames)),
		Geo:         "NG",
		MinInterval: minPollInterval,
	}
}

func (a *Adapter) Discover(ctx context.Context, leagues []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	codesBySport := map[sportSlug][]string{}
	for _, code := range leagues {
		if want, ok := defaultLeagueNames[code]; ok {
			codesBySport[want.sport] = append(codesBySport[want.sport], code)
		}
	}
	for _, sport := range slices.Sorted(maps.Keys(codesBySport)) {
		events, errs := a.sportEvents(ctx, sport, codesBySport[sport])
		snap.Events = append(snap.Events, events...)
		snap.Errors = append(snap.Errors, errs...)
	}
	if len(snap.Events) == 0 && len(snap.Errors) > 0 {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) sportEvents(ctx context.Context, sport sportSlug, codes []string) ([]canon.SourceEvent, []error) {
	var resp sportResponse
	if err := a.getJSON(ctx, fmt.Sprintf(sportPath, sport)+fieldsQuery, &resp); err != nil {
		return nil, []error{fmt.Errorf("betano %s leagues: %w", sport, err)}
	}
	listed := listedLeagues(resp.Data)
	var events []canon.SourceEvent
	var errs []error
	for _, code := range codes {
		for _, l := range a.findLeagues(code, listed) {
			found, err := a.leagueEvents(ctx, l, code, sport)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			events = append(events, found...)
		}
	}
	return events, errs
}

func listedLeagues(data sportData) []league {
	listed := slices.Clone(data.TopLeagues)
	for _, group := range data.RegionGroups {
		for _, r := range group.Regions {
			for _, l := range r.Leagues {
				if l.RegionName == "" {
					l.RegionName = r.Name
				}
				listed = append(listed, l)
			}
		}
	}
	return uniqueLeagues(listed)
}

func uniqueLeagues(listed []league) []league {
	seen := map[string]bool{}
	return slices.DeleteFunc(listed, func(l league) bool {
		duplicate := seen[l.ID]
		seen[l.ID] = true
		return duplicate
	})
}

func (a *Adapter) findLeagues(code string, listed []league) []league {
	if id, ok := a.leagueIDs[code]; ok {
		return slices.DeleteFunc(slices.Clone(listed), func(l league) bool { return l.ID != id })
	}
	want := defaultLeagueNames[code]
	return slices.DeleteFunc(slices.Clone(listed), func(l league) bool { return !want.matches(l) })
}

func (a *Adapter) leagueEvents(ctx context.Context, l league, code string, sport sportSlug) ([]canon.SourceEvent, error) {
	var resp leagueResponse
	if err := a.getJSON(ctx, "/api"+l.URL+fieldsQuery, &resp); err != nil {
		return nil, fmt.Errorf("betano league %s: %w", l.ID, err)
	}
	var events []canon.SourceEvent
	for _, b := range resp.Data.Blocks {
		for _, ev := range b.Events {
			home, away, ok := homeAndAway(ev)
			slug := eventSlug(ev.URL)
			if !ok || slug == "" {
				continue
			}
			a.rememberEvent(ev.ID, eventMeta{slug: slug, sport: sport})
			events = append(events, canon.SourceEvent{
				SourceID:   ev.ID,
				LeagueHint: code,
				Home:       home,
				Away:       away,
				Kickoff:    time.UnixMilli(ev.StartTime).UTC(),
			})
		}
	}
	return events, nil
}

func homeAndAway(ev event) (home, away string, ok bool) {
	if len(ev.Participants) == 2 {
		return ev.Participants[0].Name, ev.Participants[1].Name, true
	}
	home, away, ok = strings.Cut(ev.Name, " - ")
	return strings.TrimSpace(home), strings.TrimSpace(away), ok
}

func eventSlug(eventURL string) string {
	return path.Base(path.Dir(strings.TrimSuffix(eventURL, "/")))
}

func (a *Adapter) rememberEvent(eventID string, meta eventMeta) {
	a.metaMu.Lock()
	defer a.metaMu.Unlock()
	a.metaByEvent[eventID] = meta
}

func (a *Adapter) eventMeta(eventID string) (eventMeta, bool) {
	a.metaMu.RLock()
	defer a.metaMu.RUnlock()
	meta, ok := a.metaByEvent[eventID]
	return meta, ok
}

func (a *Adapter) Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error) {
	snap := sources.NewSnapshot(Name, a.deps.Clock())
	priced := 0
	for _, id := range eventIDs {
		meta, ok := a.eventMeta(id)
		if !ok {
			snap.Errors = append(snap.Errors, fmt.Errorf("betano event %s: %w", id, errUnknownSlug))
			continue
		}
		markets, err := a.eventMarkets(ctx, id, meta.slug)
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Errorf("betano event %s: %w", id, err))
			continue
		}
		priced++
		snap.Markets = append(snap.Markets, a.rawMarkets(id, meta.sport, markets, snap.FetchedAt)...)
	}
	if priced == 0 && len(snap.Errors) > 0 {
		return snap, errors.Join(snap.Errors...)
	}
	return snap, nil
}

func (a *Adapter) eventMarkets(ctx context.Context, eventID, slug string) ([]market, error) {
	var resp matchOddsResponse
	if err := a.getJSON(ctx, fmt.Sprintf(matchOddsPath, url.PathEscape(slug), url.PathEscape(eventID))+fieldsQuery, &resp); err != nil {
		return nil, err
	}
	if resp.Data.Event != nil && len(resp.Data.Event.Markets) > 0 {
		return resp.Data.Event.Markets, nil
	}
	return resp.Data.Markets, nil
}

func (a *Adapter) getJSON(ctx context.Context, pathAndQuery string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return a.deps.GetJSON(ctx, a.deps.Config.BaseURL+pathAndQuery, out)
}
