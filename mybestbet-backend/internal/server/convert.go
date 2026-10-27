package server

import "mybestbet/internal/stats"

func hitRateDTO(r stats.HitRate) HitRate {
	return HitRate{Window: r.Window, Matches: r.Matches, Hits: r.Hits}
}

func hitRatesDTO(rates []stats.HitRate) []HitRate {
	out := make([]HitRate, len(rates))
	for i, r := range rates {
		out[i] = hitRateDTO(r)
	}
	return out
}

func matchStatsDTO(matches []stats.MatchStat) []PlayerMatchStat {
	out := make([]PlayerMatchStat, len(matches))
	for i, m := range matches {
		out[i] = PlayerMatchStat{
			EventID:        m.EventID,
			TeamID:         m.TeamID,
			OpponentTeamID: m.OpponentTeamID,
			IsHome:         m.IsHome,
			PlayedAt:       m.PlayedAt,
			Minutes:        m.Minutes,
			Started:        m.Started,
			Goals:          m.Goals,
			Assists:        m.Assists,
			Shots:          m.Shots,
			ShotsOnTarget:  m.ShotsOnTarget,
			Fouls:          m.Fouls,
			Tackles:        m.Tackles,
			Yellow:         m.Yellow,
			Red:            m.Red,
			Rating:         m.Rating,
			Stats:          m.StatValues(),
		}
	}
	return out
}

func minutesDTO(points []stats.MinutesPoint) []MinutesPoint {
	out := make([]MinutesPoint, len(points))
	for i, p := range points {
		out[i] = MinutesPoint{EventID: p.EventID, PlayedAt: p.PlayedAt, Minutes: p.Minutes, Started: p.Started}
	}
	return out
}

func ratingDistributionDTO(d stats.RatingDistribution) *RatingDistribution {
	buckets := make([]RatingBucket, len(d.Buckets))
	for i, b := range d.Buckets {
		buckets[i] = RatingBucket{From: b.From, To: b.To, Count: b.Count}
	}
	return &RatingDistribution{Ratings: emptyIfNil(d.Ratings), Mean: d.Mean, Min: d.Min, Max: d.Max, Buckets: buckets}
}
