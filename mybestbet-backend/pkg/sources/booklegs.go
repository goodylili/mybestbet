package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
	"net/http"
	"strings"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
)

var (
	ErrLegNotOffered   = errors.New("selection not offered")
	ErrLegSuspended    = errors.New("selection suspended")
	ErrLegEventMissing = errors.New("event not found")
	ErrLegEventFailed  = errors.New("event unavailable")
	ErrNothingBookable = errors.New("no bookable legs")
)

const lineTolerance = 1e-6

func (l BookLeg) matchesMarket(m canon.RawMarket) bool {
	return m.Code == l.Code && m.Period == l.Period && sameLine(m.Line, l.Line) && samePlayer(m.Player, l.PlayerName)
}

func (l BookLeg) matchesSelection(s canon.Selection) bool {
	return s.Side == canon.Side(l.Side) && s.Label == l.Label
}

func sameLine(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Abs(*a-*b) < lineTolerance
}

func samePlayer(p *canon.SourcePlayer, name string) bool {
	if name == "" {
		return p == nil
	}
	return p != nil && strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name))
}

func PickLegOutcome[R any](outcomes iter.Seq2[R, []canon.RawMarket], leg BookLeg) (R, float64, error) {
	var zero R
	suspended := false
	for ref, markets := range outcomes {
		for _, m := range markets {
			if !leg.matchesMarket(m) {
				continue
			}
			for _, s := range m.Selections {
				if !leg.matchesSelection(s) {
					continue
				}
				if s.Price.Suspended {
					suspended = true
					continue
				}
				return ref, s.Price.Raw, nil
			}
		}
	}
	if suspended {
		return zero, 0, ErrLegSuspended
	}
	return zero, 0, ErrLegNotOffered
}

type LegEventFetcher[E any] func(ctx context.Context, sourceEventID string) (E, error)

type LegPicker[E, R any] func(event E, leg BookLeg) (R, float64, error)

type ResolvedLegs[R any] struct {
	Refs    []R
	Indexes []int
	Legs    []BookedLeg
}

func (r ResolvedLegs[R]) Reject(refIndex int, reason string) {
	leg := &r.Legs[r.Indexes[refIndex]]
	leg.OK = false
	leg.Reason = reason
}

func ResolveLegs[E, R any](ctx context.Context, legs []BookLeg, fetch LegEventFetcher[E], pick LegPicker[E, R]) (ResolvedLegs[R], error) {
	resolved := ResolvedLegs[R]{Legs: make([]BookedLeg, len(legs))}
	events := map[string]E{}
	eventErrs := map[string]error{}
	var failures []error
	for i, leg := range legs {
		resolved.Legs[i] = BookedLeg{Index: i}
		ref, price, err := resolveLeg(ctx, leg, events, eventErrs, fetch, pick)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return resolved, ctxErr
		}
		if err != nil {
			resolved.Legs[i].Reason = legReason(err)
			failures = append(failures, fmt.Errorf("leg %d: %w", i, err))
			continue
		}
		resolved.Legs[i].OK = true
		resolved.Legs[i].Price = price
		resolved.Refs = append(resolved.Refs, ref)
		resolved.Indexes = append(resolved.Indexes, i)
	}
	if len(resolved.Refs) == 0 {
		return resolved, fmt.Errorf("%w: %w", ErrNothingBookable, errors.Join(failures...))
	}
	return resolved, nil
}

func resolveLeg[E, R any](
	ctx context.Context,
	leg BookLeg,
	events map[string]E,
	eventErrs map[string]error,
	fetch LegEventFetcher[E],
	pick LegPicker[E, R],
) (R, float64, error) {
	var zero R
	if err, failed := eventErrs[leg.SourceEventID]; failed {
		return zero, 0, err
	}
	event, fetched := events[leg.SourceEventID]
	if !fetched {
		var err error
		event, err = fetch(ctx, leg.SourceEventID)
		if err != nil {
			eventErrs[leg.SourceEventID] = legEventError(err)
			return zero, 0, eventErrs[leg.SourceEventID]
		}
		events[leg.SourceEventID] = event
	}
	return pick(event, leg)
}

func legEventError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrLegEventMissing, err)
	}
	return fmt.Errorf("%w: %w", ErrLegEventFailed, err)
}

var legReasons = []error{ErrLegNotOffered, ErrLegSuspended, ErrLegEventMissing, ErrLegEventFailed}

func legReason(err error) string {
	for _, known := range legReasons {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrLegNotOffered.Error()
}

func (d Deps) PostJSON(ctx context.Context, url string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encode %s: %w", url, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request %s: %w", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.Do(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(resp, out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func BookingFailure(err error) error {
	if errors.Is(err, ErrForbidden) {
		return fmt.Errorf("%w: %w", ErrBookingBlocked, err)
	}
	return err
}
