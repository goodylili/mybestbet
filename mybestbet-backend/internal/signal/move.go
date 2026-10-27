package signal

import (
	"math"
	"time"

	"mybestbet/pkg/canon"
)

const MinMoveFraction = 0.05

type MovePoint struct {
	raw             float64
	prob            float64
	observedAtNanos int64
	seenAtNanos     int64
	suspended       bool
}

func NewMovePoint(p canon.Price, seenAt time.Time) MovePoint {
	return MovePoint{raw: p.Raw, prob: p.Prob, observedAtNanos: p.ObservedAt.UnixNano(), seenAtNanos: seenAt.UnixNano(), suspended: p.Suspended}
}

func (p MovePoint) observedAt() time.Time { return time.Unix(0, p.observedAtNanos).UTC() }

func (p MovePoint) seenAt() time.Time { return time.Unix(0, p.seenAtNanos).UTC() }

func (p MovePoint) priceIn(form canon.PriceForm) canon.Price {
	return canon.Price{Raw: p.raw, Form: form, Prob: p.prob, ObservedAt: p.observedAt()}
}

func TrimMoveWindow(points []MovePoint, cutoff time.Time) []MovePoint {
	firstKept := 0
	for firstKept < len(points) && points[firstKept].observedAtNanos < cutoff.UnixNano() {
		firstKept++
	}
	return points[firstKept:]
}

func DetectMove(key canon.MarketKey, current Quote, history []MovePoint, minHold time.Duration, now time.Time) (canon.Signal, bool) {
	if len(history) < 2 || !isMovable(current) {
		return canon.Signal{}, false
	}
	latest := history[len(history)-1]
	if latest.raw != current.Price.Raw || now.Sub(latest.seenAt()) < minHold {
		return canon.Signal{}, false
	}
	var start canon.Price
	var largest float64
	for i, p := range history[:len(history)-1] {
		if !outlivedItsSnapshot(p, history[i+1]) || latest.observedAtNanos <= p.observedAtNanos {
			continue
		}
		if move := math.Abs(current.Price.Prob-p.prob) / p.prob; move > largest {
			largest, start = move, p.priceIn(current.Price.Form)
		}
	}
	if largest < MinMoveFraction {
		return canon.Signal{}, false
	}
	direction := "down"
	if current.Price.Prob > start.Prob {
		direction = "up"
	}
	from := current
	from.Price = start
	s := newSignal(canon.SignalMove, key, []canon.Leg{from.leg(0), current.leg(0)}, now, current.Source, current.outcomeTag(), direction)
	s.Margin = largest
	DescribeMove(&s)
	return s, true
}

func DescribeMove(s *canon.Signal) {
	if s.Type != canon.SignalMove || len(s.Legs) != 2 {
		return
	}
	s.FromPrice, s.ToPrice = &s.Legs[0].Price, &s.Legs[1].Price
	s.Change = s.ToPrice.Prob - s.FromPrice.Prob
}

func isMovable(q Quote) bool {
	return !q.Derived && !q.Degraded && !q.Stale && !q.Price.Suspended && q.Price.Form != canon.FormLineOnly && q.Price.Prob > 0
}

func outlivedItsSnapshot(p, next MovePoint) bool {
	return p.prob > 0 && !p.suspended && next.observedAtNanos > p.observedAtNanos
}
