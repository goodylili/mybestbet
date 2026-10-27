package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/sources"
)

type eventLinks struct {
	linkers map[string]sources.EventLinker
	leagues map[string]config.League
}

func newEventLinks(cfg config.Config) eventLinks {
	links := eventLinks{linkers: map[string]sources.EventLinker{}, leagues: make(map[string]config.League, len(cfg.Leagues))}
	for name, sc := range cfg.Sources {
		factory, ok := sources.Lookup(name)
		if !ok || !sc.Enabled {
			continue
		}
		if linker, ok := factory(sources.Deps{Config: sc}).(sources.EventLinker); ok {
			links.linkers[name] = linker
		}
	}
	for _, l := range cfg.Leagues {
		links.leagues[l.Code] = l
	}
	return links
}

func (l eventLinks) url(source, sourceEventID, sourceLeagueID string, flipped bool, event EventInfo) string {
	linker, ok := l.linkers[source]
	if !ok {
		return ""
	}
	league := l.leagues[event.LeagueCode]
	home, away := event.Home.Name, event.Away.Name
	if flipped {
		home, away = away, home
	}
	return linker.EventURL(sources.EventRef{
		SourceEventID:  sourceEventID,
		SourceLeagueID: sourceLeagueID,
		Sport:          event.Sport,
		League:         event.LeagueCode,
		Country:        league.Country,
		LeagueName:     league.Name,
		Home:           home,
		Away:           away,
	})
}

func (s *Server) attachEventLinks(ctx context.Context, events []EventInfo) error {
	if len(events) == 0 || len(s.links.linkers) == 0 {
		return nil
	}
	ids := make([]string, len(events))
	positions := make(map[string]int, len(events))
	for i, e := range events {
		ids[i] = e.ID
		positions[e.ID] = i
	}
	rows, err := s.pool.Query(ctx, `SELECT "eventId"::text, source, "sourceEventId", "sourceLeagueId", flipped FROM event_links
		WHERE "eventId" = ANY($1::uuid[]) ORDER BY "createdAt", "sourceEventId"`, ids)
	if err != nil {
		return fmt.Errorf("list event links: %w", err)
	}
	var eventID, source, sourceEventID, sourceLeagueID string
	var flipped bool
	unlinked := map[string][]string{}
	_, err = pgx.ForEachRow(rows, []any{&eventID, &source, &sourceEventID, &sourceLeagueID, &flipped}, func() error {
		i, ok := positions[eventID]
		if !ok {
			return nil
		}
		event := &events[i]
		if event.Sport == "" {
			event.Sport = s.sportOf(event.LeagueCode)
		}
		if link := s.links.url(source, sourceEventID, sourceLeagueID, flipped, *event); link != "" {
			setLink(event, source, link)
		} else if _, linkable := s.links.linkers[source]; linkable {
			unlinked[eventID] = append(unlinked[eventID], source)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan event links: %w", err)
	}
	return s.attachMarketLinks(ctx, events, positions, unlinked)
}

func (s *Server) attachMarketLinks(ctx context.Context, events []EventInfo, positions map[string]int, unlinked map[string][]string) error {
	if len(unlinked) == 0 {
		return nil
	}
	ids := make([]string, 0, len(unlinked))
	for id := range unlinked {
		ids = append(ids, id)
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON ("eventId", source) "eventId"::text, source, "deepLink" FROM prices_current
		WHERE "eventId" = ANY($1::uuid[]) AND "deepLink" <> ''
		ORDER BY "eventId", source, code IN ('1X2', 'ML') DESC, period = '' DESC, "playerId" IS NULL DESC, "marketKey"`, ids)
	if err != nil {
		return fmt.Errorf("list market links: %w", err)
	}
	var eventID, source, link string
	_, err = pgx.ForEachRow(rows, []any{&eventID, &source, &link}, func() error {
		if i, ok := positions[eventID]; ok && slices.Contains(unlinked[eventID], source) && events[i].Links[source] == "" {
			setLink(&events[i], source, link)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan market links: %w", err)
	}
	return nil
}

func setLink(event *EventInfo, source, link string) {
	if event.Links == nil {
		event.Links = map[string]string{}
	}
	event.Links[source] = link
}

func (s *Server) indexedLinks(eventID string) map[string]string {
	if index, _ := s.events.get(); index != nil {
		return index.byID[eventID].Links
	}
	return nil
}

func (s *Server) attachSignalLinks(signals []SignalView) {
	for i := range signals {
		links := s.indexedLinks(signals[i].EventID)
		if links == nil {
			continue
		}
		signals[i].Event.Links = links
		for j := range signals[i].Legs {
			if leg := &signals[i].Legs[j]; leg.DeepLink == "" {
				leg.DeepLink = links[leg.Source]
			}
		}
	}
}
