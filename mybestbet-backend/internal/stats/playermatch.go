package stats

import "mybestbet/internal/names"

type nameMatch struct {
	playerID string
	isExact  bool
}

func matchPlayerByName(candidates []NamedPlayer, name, teamHint string) (nameMatch, bool) {
	full := names.Person(name).Full()
	exact := filterPlayers(candidates, func(p NamedPlayer) bool { return names.Person(p.Name).Full() == full })
	if id, ok := uniqueWithinTeam(exact, teamHint); ok {
		return nameMatch{playerID: id, isExact: true}, true
	}
	key := names.Person(name).SurnameInitialKey()
	if key == "" {
		return nameMatch{}, false
	}
	loose := filterPlayers(candidates, func(p NamedPlayer) bool { return names.Person(p.Name).SurnameInitialKey() == key })
	if id, ok := uniqueWithinTeam(loose, teamHint); ok {
		return nameMatch{playerID: id}, true
	}
	return nameMatch{}, false
}

func uniqueWithinTeam(players []NamedPlayer, teamHint string) (string, bool) {
	if len(players) > 1 && teamHint != "" {
		players = filterPlayers(players, func(p NamedPlayer) bool { return names.TeamsLikelySame(p.TeamName, teamHint) })
	}
	if len(players) != 1 {
		return "", false
	}
	return players[0].ID, true
}

func filterPlayers(players []NamedPlayer, keep func(NamedPlayer) bool) []NamedPlayer {
	var kept []NamedPlayer
	for _, p := range players {
		if keep(p) {
			kept = append(kept, p)
		}
	}
	return kept
}
