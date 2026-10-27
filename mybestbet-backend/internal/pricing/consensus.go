package pricing

import (
	"cmp"
	"slices"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

type WeightedProb struct {
	Feed   string
	Prob   float64
	Weight float64
}

var depthWeightedSources = map[string]bool{"polymarket": true, "kalshi": true}

const medianTieTolerance = 1e-12

func SourceWeight(cfg config.Config, source string, touch canon.Price) float64 {
	if !depthWeightedSources[source] {
		return cfg.Weight(source)
	}
	if touch.Liquidity != nil && *touch.Liquidity > cfg.Pricing.DeepBookLiquidity && cfg.Pricing.DeepBookWeight > 0 {
		return cfg.Pricing.DeepBookWeight
	}
	return 1
}

func WeightedMedian(values []WeightedProb) float64 {
	sorted := slices.Clone(values)
	slices.SortFunc(sorted, func(a, b WeightedProb) int { return cmp.Compare(a.Prob, b.Prob) })
	var total float64
	for _, v := range sorted {
		total += v.Weight
	}
	half := total / 2
	var cumulative float64
	for i, v := range sorted {
		cumulative += v.Weight
		if cumulative-half > medianTieTolerance {
			return v.Prob
		}
		if cumulative-half >= -medianTieTolerance && i+1 < len(sorted) {
			return (v.Prob + sorted[i+1].Prob) / 2
		}
	}
	if len(sorted) == 0 {
		return 0
	}
	return sorted[len(sorted)-1].Prob
}

func Consensus(values []WeightedProb, minFeeds int) (float64, bool) {
	votes := oneVotePerFeed(values)
	if len(votes) < minFeeds || len(votes) == 0 {
		return 0, false
	}
	return WeightedMedian(votes), true
}

func oneVotePerFeed(values []WeightedProb) []WeightedProb {
	voteIndex := make(map[string]int, len(values))
	members := make([]int, 0, len(values))
	votes := make([]WeightedProb, 0, len(values))
	for _, v := range values {
		i, seen := voteIndex[v.Feed]
		if !seen {
			voteIndex[v.Feed] = len(votes)
			votes = append(votes, v)
			members = append(members, 1)
			continue
		}
		members[i]++
		votes[i].Prob += (v.Prob - votes[i].Prob) / float64(members[i])
		votes[i].Weight = max(votes[i].Weight, v.Weight)
	}
	return votes
}
