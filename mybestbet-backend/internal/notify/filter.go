package notify

import (
	"slices"
	"time"

	"mybestbet/pkg/canon"
)

const percent = 100

func (s Subscription) isMuted(now time.Time) bool {
	return s.MutedUntil != nil && now.Before(*s.MutedUntil)
}

func (s Subscription) wants(signal canon.Signal, sport canon.Sport, now time.Time) bool {
	if signal.Status != canon.SignalOpen || s.isMuted(now) {
		return false
	}
	if len(s.Leagues) > 0 && !slices.Contains(s.Leagues, signal.LeagueCode) {
		return false
	}
	if len(s.Sports) > 0 && !slices.Contains(s.Sports, string(sport)) {
		return false
	}
	isProp := signal.Market.Code.IsPlayerProp()
	if isProp && !s.Props {
		return false
	}
	strength := signalStrength(signal) * percent
	switch signal.Type {
	case canon.SignalArb:
		return strength >= s.MinArbPercent
	case canon.SignalEV:
		return strength >= s.MinEVPercent
	case canon.SignalMove:
		return s.MovePercent != nil && strength >= *s.MovePercent
	case canon.SignalLine:
		return isProp
	}
	return false
}

func sportOf(signal canon.Signal, leagueSports map[string]canon.Sport) canon.Sport {
	if sport := leagueSports[signal.LeagueCode]; sport != "" {
		return sport
	}
	if spec, ok := canon.SpecOf(signal.Market.Code); ok && len(spec.Sports) == 1 {
		return spec.Sports[0]
	}
	return ""
}

func signalStrength(signal canon.Signal) float64 {
	if signal.Type == canon.SignalEV {
		return signal.EV
	}
	return signal.Margin
}
