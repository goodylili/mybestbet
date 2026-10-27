package sporty

import (
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
)

const (
	sportradarPlayerPrefix = "sr:player:"
	playerPropsPrefix      = "pre:playerprops:"
)

type playerOutcomeRef struct {
	playerID string
	atLeast  int
}

func parsePlayerOutcomeID(outcomeID string) (playerOutcomeRef, bool) {
	if id, ok := strings.CutPrefix(outcomeID, sportradarPlayerPrefix); ok {
		return playerOutcomeRef{playerID: sportradarPlayerPrefix + id}, id != ""
	}
	rest, ok := strings.CutPrefix(outcomeID, playerPropsPrefix)
	if !ok {
		return playerOutcomeRef{}, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 || parts[1] == "" {
		return playerOutcomeRef{}, false
	}
	ref := playerOutcomeRef{playerID: sportradarPlayerPrefix + parts[1]}
	if len(parts) > 2 {
		if atLeast, err := strconv.Atoi(parts[2]); err == nil {
			ref.atLeast = atLeast
		}
	}
	return ref, true
}

func playerFromDesc(playerID, desc string) *canon.SourcePlayer {
	nameAndTeam := strings.TrimSpace(withoutThresholdSuffix(desc))
	player := &canon.SourcePlayer{SourceID: playerID, Name: nameAndTeam}
	open := strings.LastIndex(nameAndTeam, " (")
	if open >= 0 && strings.HasSuffix(nameAndTeam, ")") {
		player.Team = nameAndTeam[open+2 : len(nameAndTeam)-1]
		player.Name = nameAndTeam[:open]
	}
	player.Name = givenNameFirst(player.Name)
	return player
}

func givenNameFirst(name string) string {
	if last, first, isSurnameFirst := strings.Cut(name, ", "); isSurnameFirst {
		return first + " " + last
	}
	return name
}

const playerStatSeparator = " total "

func playerFromSpecifier(m market) (*canon.SourcePlayer, bool) {
	playerID, ok := m.specifierValue(playerSpecifier)
	if !ok || !strings.HasPrefix(playerID, sportradarPlayerPrefix) {
		return nil, false
	}
	name, _, ok := strings.Cut(m.Desc, playerStatSeparator)
	if !ok || name == "" {
		return nil, false
	}
	return &canon.SourcePlayer{SourceID: playerID, Name: givenNameFirst(name)}, true
}

func withoutThresholdSuffix(desc string) string {
	cut := strings.LastIndex(desc, " ")
	if cut < 0 || !strings.HasSuffix(desc, "+") {
		return desc
	}
	return desc[:cut]
}
