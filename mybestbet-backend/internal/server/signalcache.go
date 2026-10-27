package server

import (
	"context"
	"slices"
	"time"

	"mybestbet/pkg/canon"
)

const (
	signalSnapshotRefresh = 2 * time.Second
	maxCachedSignals      = 5000
)

type signalSnapshot struct {
	signals []SignalView
	badges  map[string]Badges
}

func (s *Server) refreshSignals(ctx context.Context) error {
	signals, err := s.openSignals(ctx, signalFilter{limit: maxCachedSignals})
	if err != nil {
		return err
	}
	s.signals.set(signalSnapshot{signals: signals, badges: badgesOf(signals)})
	return nil
}

func badgesOf(signals []SignalView) map[string]Badges {
	badges := map[string]Badges{}
	for _, sig := range signals {
		b := badges[sig.EventID]
		switch sig.Type {
		case canon.SignalArb:
			b.Arbs++
			b.MaxMargin = max(b.MaxMargin, sig.Margin)
		case canon.SignalEV:
			b.EVs++
			b.MaxEV = max(b.MaxEV, sig.EV)
		default:
			continue
		}
		badges[sig.EventID] = b
	}
	return badges
}

func (f signalFilter) matches(sig SignalView) bool {
	if f.signalType != "" && string(sig.Type) != f.signalType {
		return false
	}
	if max(sig.Margin, sig.EV) < f.minimum {
		return false
	}
	return f.leagueCodes == nil || slices.Contains(f.leagueCodes, sig.LeagueCode)
}

func (s *Server) cachedSignals(ctx context.Context, filter signalFilter) ([]SignalView, error) {
	cached, ok := s.signals.get()
	if !ok {
		return s.openSignals(ctx, filter)
	}
	signals := make([]SignalView, 0, min(filter.limit, len(cached.signals)))
	for _, sig := range cached.signals {
		if len(signals) == filter.limit {
			break
		}
		if filter.matches(sig) {
			signals = append(signals, sig)
		}
	}
	return signals, nil
}

func (s *Server) cachedBadges(ctx context.Context, eventIDs []string) (map[string]Badges, error) {
	if cached, ok := s.signals.get(); ok {
		return cached.badges, nil
	}
	return s.badgesByEvent(ctx, eventIDs)
}
