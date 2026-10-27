package resolve

import (
	"mybestbet/internal/names"
	"slices"
	"strings"

	"github.com/xrash/smetrics"
)

const (
	jaroWinklerBoostThreshold = 0.7
	jaroWinklerPrefixSize     = 4
	tokenSubsetScore          = 0.95
	ambiguityMargin           = 0.02
	reviewCandidateCount      = 3
	distinctSchoolToken       = "state"
)

type scoredCandidate struct {
	id    string
	name  string
	score float64
}

func jaroWinkler(a, b string) float64 {
	if a == b {
		return 1
	}
	return smetrics.JaroWinkler(a, b, jaroWinklerBoostThreshold, jaroWinklerPrefixSize)
}

func teamSimilarity(a, b string, allowSingleTokenSubset bool) float64 {
	if !names.HaveSameSquadQualifiers(a, b) {
		return 0
	}
	if isTokenSubset(a, b, allowSingleTokenSubset) {
		return max(tokenSubsetScore, jaroWinkler(a, b))
	}
	return jaroWinkler(a, b)
}

func isTokenSubset(a, b string, allowSingleToken bool) bool {
	shorter, longer := strings.Fields(a), strings.Fields(b)
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) == 0 || (len(shorter) == 1 && !allowSingleToken) {
		return false
	}
	for _, token := range shorter {
		if !slices.Contains(longer, token) {
			return false
		}
	}
	return !slices.Contains(longer, distinctSchoolToken) || slices.Contains(shorter, distinctSchoolToken)
}

func personSimilarity(a, b names.PersonName) float64 {
	return max(jaroWinkler(a.Full(), b.Full()), jaroWinkler(a.SortedFull(), b.SortedFull()))
}

func isClearWinner(ranked []scoredCandidate, threshold float64) bool {
	if len(ranked) == 0 || ranked[0].score < threshold {
		return false
	}
	return len(ranked) == 1 || ranked[1].score < ranked[0].score-ambiguityMargin
}
