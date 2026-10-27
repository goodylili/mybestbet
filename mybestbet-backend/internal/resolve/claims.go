package resolve

type storedPlayerAlias struct {
	source     string
	sourceID   string
	playerID   string
	confidence float64
}

func uncontestedPlayerAliases(aliases []storedPlayerAlias) []storedPlayerAlias {
	byClaim := map[playerClaim][]storedPlayerAlias{}
	for _, a := range aliases {
		claim := playerClaim{source: a.source, playerID: a.playerID}
		byClaim[claim] = append(byClaim[claim], a)
	}
	kept := make([]storedPlayerAlias, 0, len(aliases))
	for _, a := range aliases {
		if isTrustedClaim(byClaim[playerClaim{source: a.source, playerID: a.playerID}], a) {
			kept = append(kept, a)
		}
	}
	return kept
}

func isTrustedClaim(rivals []storedPlayerAlias, a storedPlayerAlias) bool {
	if len(rivals) == 1 {
		return true
	}
	exact := 0
	for _, r := range rivals {
		if r.confidence >= exactPlayerNameScore {
			exact++
		}
	}
	return exact == 1 && a.confidence >= exactPlayerNameScore
}
