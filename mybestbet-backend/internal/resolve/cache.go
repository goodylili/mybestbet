package resolve

import (
	"slices"
	"sync"
	"time"

	"mybestbet/internal/names"
	"mybestbet/pkg/canon"
)

type aliasKey struct {
	source    string
	sourceKey string
}

type playerClaim struct {
	source   string
	playerID string
}

type teamName struct {
	teamID     string
	normalized string
	literal    string
	raw        string
}

type squadMembership struct {
	teamID   string
	playerID string
}

type teamPair struct {
	low  string
	high string
}

func pairOf(a, b string) teamPair {
	if a > b {
		a, b = b, a
	}
	return teamPair{low: a, high: b}
}

type squadPlayer struct {
	Player
	name names.PersonName
}

type Cache struct {
	mu               sync.RWMutex
	matching         MatchingConfig
	leagueSports     map[string]canon.Sport
	teams            map[string]Team
	teamNames        map[string][]teamName
	leagueTeams      map[string][]string
	teamLeagues      map[string][]string
	teamAliases      map[aliasKey]string
	events           map[string]Event
	eventsByTeamPair map[teamPair][]string
	eventsByTeam     map[string][]string
	eventLinks       map[aliasKey]EventLink
	players          map[string]Player
	squads           map[string][]squadPlayer
	squadMembers     map[squadMembership]bool
	playerAliases    map[aliasKey]string
	playerClaimants  map[playerClaim]string
}

func NewCache() (*Cache, error) {
	if err := names.LoadError(); err != nil {
		return nil, err
	}
	return &Cache{
		matching:         DefaultMatchingConfig,
		leagueSports:     map[string]canon.Sport{},
		teams:            map[string]Team{},
		teamNames:        map[string][]teamName{},
		leagueTeams:      map[string][]string{},
		teamLeagues:      map[string][]string{},
		teamAliases:      map[aliasKey]string{},
		events:           map[string]Event{},
		eventsByTeamPair: map[teamPair][]string{},
		eventsByTeam:     map[string][]string{},
		eventLinks:       map[aliasKey]EventLink{},
		players:          map[string]Player{},
		squads:           map[string][]squadPlayer{},
		squadMembers:     map[squadMembership]bool{},
		playerAliases:    map[aliasKey]string{},
		playerClaimants:  map[playerClaim]string{},
	}, nil
}

func (c *Cache) setMatching(m MatchingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.matching = m
}

func (c *Cache) addLeague(code string, sport canon.Sport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.leagueSports[code] = sport
}

func (c *Cache) addTeam(t Team) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, known := c.teams[t.ID]; known {
		return
	}
	c.teams[t.ID] = t
	c.addTeamNameLocked(t.ID, t.Name)
	c.joinLeagueLocked(t.ID, t.League)
}

func (c *Cache) addTeamNameLocked(teamID, raw string) {
	name := teamName{teamID: teamID, normalized: names.Team(raw), literal: names.TeamLiteral(raw), raw: raw}
	if !slices.Contains(c.teamNames[teamID], name) {
		c.teamNames[teamID] = append(c.teamNames[teamID], name)
	}
}

func (c *Cache) joinLeague(teamID, league string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.joinLeagueLocked(teamID, league)
}

func (c *Cache) joinLeagueLocked(teamID, league string) {
	if league == "" || slices.Contains(c.teamLeagues[teamID], league) {
		return
	}
	c.teamLeagues[teamID] = append(c.teamLeagues[teamID], league)
	c.leagueTeams[league] = append(c.leagueTeams[league], teamID)
}

func (c *Cache) addTeamAlias(source, sourceName, teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.teamAliases[aliasKey{source, sourceName}] = teamID
	c.addTeamNameLocked(teamID, sourceName)
}

func (c *Cache) addEvent(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, known := c.events[e.ID]; known {
		return
	}
	c.events[e.ID] = e
	pair := pairOf(e.HomeTeamID, e.AwayTeamID)
	c.eventsByTeamPair[pair] = append(c.eventsByTeamPair[pair], e.ID)
	c.eventsByTeam[e.HomeTeamID] = append(c.eventsByTeam[e.HomeTeamID], e.ID)
	c.eventsByTeam[e.AwayTeamID] = append(c.eventsByTeam[e.AwayTeamID], e.ID)
}

func (c *Cache) mergeEvent(duplicate string, into EventLink) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, link := range c.eventLinks {
		if link.EventID == duplicate {
			c.eventLinks[key] = EventLink{EventID: into.EventID, Flipped: link.Flipped != into.Flipped}
		}
	}
	e, known := c.events[duplicate]
	if !known {
		return
	}
	delete(c.events, duplicate)
	isDuplicate := func(id string) bool { return id == duplicate }
	pair := pairOf(e.HomeTeamID, e.AwayTeamID)
	c.eventsByTeamPair[pair] = slices.DeleteFunc(c.eventsByTeamPair[pair], isDuplicate)
	c.eventsByTeam[e.HomeTeamID] = slices.DeleteFunc(c.eventsByTeam[e.HomeTeamID], isDuplicate)
	c.eventsByTeam[e.AwayTeamID] = slices.DeleteFunc(c.eventsByTeam[e.AwayTeamID], isDuplicate)
}

func (c *Cache) linkEvent(source, sourceEventID string, link EventLink) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventLinks[aliasKey{source, sourceEventID}] = link
}

func (c *Cache) addPlayer(p Player) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, known := c.players[p.ID]; known {
		return
	}
	c.players[p.ID] = p
	c.addSquadMemberLocked(p.TeamID, p.ID)
}

func (c *Cache) addSquadMember(teamID, playerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addSquadMemberLocked(teamID, playerID)
}

func (c *Cache) addSquadMemberLocked(teamID, playerID string) {
	p, known := c.players[playerID]
	membership := squadMembership{teamID: teamID, playerID: playerID}
	if teamID == "" || !known || c.squadMembers[membership] {
		return
	}
	c.squadMembers[membership] = true
	c.squads[teamID] = append(c.squads[teamID], squadPlayer{Player: p, name: names.Person(p.Name)})
}

func (c *Cache) addPlayerAlias(source, sourceID, playerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playerAliases[aliasKey{source, sourceID}] = playerID
	c.playerClaimants[playerClaim{source, playerID}] = sourceID
}

func (c *Cache) mergePlayer(duplicateID, survivorID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.players, duplicateID)
	for membership := range c.squadMembers {
		if membership.playerID != duplicateID {
			continue
		}
		delete(c.squadMembers, membership)
		c.squads[membership.teamID] = slices.DeleteFunc(c.squads[membership.teamID], func(sp squadPlayer) bool { return sp.ID == duplicateID })
	}
	for key, playerID := range c.playerAliases {
		if playerID == duplicateID {
			c.playerAliases[key] = survivorID
		}
	}
	for claim, sourceKey := range c.playerClaimants {
		if claim.playerID != duplicateID {
			continue
		}
		delete(c.playerClaimants, claim)
		survivorClaim := playerClaim{source: claim.source, playerID: survivorID}
		if _, claimed := c.playerClaimants[survivorClaim]; !claimed {
			c.playerClaimants[survivorClaim] = sourceKey
		}
	}
}

func (c *Cache) team(id string) (Team, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.teams[id]
	return t, ok
}

func (c *Cache) event(id string) (Event, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.events[id]
	return e, ok
}

func (c *Cache) player(id string) (Player, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.players[id]
	return p, ok
}

func (c *Cache) hasPlayer(id string) bool {
	_, ok := c.player(id)
	return ok
}

func (c *Cache) sportOf(league string) canon.Sport {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leagueSports[league]
}

func (c *Cache) kickoffToleranceIn(league string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.matching.KickoffToleranceFor(c.leagueSports[league])
}

func (c *Cache) isCollegeLeague(league string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.matching.IsCollegeLeague(league)
}

func (c *Cache) isKnownLeague(league string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, known := c.leagueSports[league]
	return known || len(c.leagueTeams[league]) > 0
}

func (c *Cache) hasLeagueTeams(league string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.leagueTeams[league]) > 0
}

func (c *Cache) isLeagueMember(teamID, league string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Contains(c.teamLeagues[teamID], league)
}

func (c *Cache) isSportMember(teamID string, sport canon.Sport) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, league := range c.teamLeagues[teamID] {
		if c.leagueSports[league] == sport {
			return true
		}
	}
	return false
}

func (c *Cache) teamAlias(source, sourceName string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.teamAliases[aliasKey{source, sourceName}]
	return id, ok
}

func (c *Cache) eventLink(source, sourceEventID string) (EventLink, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	link, ok := c.eventLinks[aliasKey{source, sourceEventID}]
	return link, ok
}

func (c *Cache) playerAlias(source, sourceID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.playerAliases[aliasKey{source, sourceID}]
	return id, ok
}

func (c *Cache) playerClaimant(source, playerID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sourceKey, ok := c.playerClaimants[playerClaim{source, playerID}]
	return sourceKey, ok
}

func (c *Cache) leagueNames(league string) []teamName {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var pool []teamName
	for _, id := range c.leagueTeams[league] {
		pool = append(pool, c.teamNames[id]...)
	}
	return pool
}

func (c *Cache) sportNames(sport canon.Sport) []teamName {
	c.mu.RLock()
	defer c.mu.RUnlock()
	seen := map[string]bool{}
	var pool []teamName
	for league, leagueSport := range c.leagueSports {
		if leagueSport != sport {
			continue
		}
		for _, id := range c.leagueTeams[league] {
			if !seen[id] {
				seen[id] = true
				pool = append(pool, c.teamNames[id]...)
			}
		}
	}
	return pool
}

func (c *Cache) eventsBetween(teamA, teamB string) []Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := c.eventsByTeamPair[pairOf(teamA, teamB)]
	events := make([]Event, 0, len(ids))
	for _, id := range ids {
		events = append(events, c.events[id])
	}
	return events
}

func (c *Cache) eventsOf(teamID string) []Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := c.eventsByTeam[teamID]
	events := make([]Event, 0, len(ids))
	for _, id := range ids {
		events = append(events, c.events[id])
	}
	return events
}

func (c *Cache) namesOf(teamID string) []teamName {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.teamNames[teamID])
}

func (c *Cache) squadsOf(teamIDs ...string) []squadPlayer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var squad []squadPlayer
	for _, id := range teamIDs {
		squad = append(squad, c.squads[id]...)
	}
	return squad
}
