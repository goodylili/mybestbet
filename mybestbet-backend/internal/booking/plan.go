package booking

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"

	"mybestbet/internal/resolve"
	"mybestbet/pkg/sources"
)

const (
	reasonNotListed      = "event not listed at this book"
	reasonTeamMarket     = "team stat markets cannot be booked"
	reasonNoCodes        = "this source has no booking codes, place it manually"
	reasonNotReturned    = "the book did not return this selection"
	messageUnsupported   = "booking codes are not available for this book"
	messageBlocked       = "this book blocks booking from our region"
	messageFailed        = "the book did not return a code, try again"
	messageNothingToBook = "none of the selections are available at this book"
)

type plannedLeg struct {
	index int
	leg   sources.BookLeg
}

type bookPlan struct {
	book     string
	booker   sources.Booker
	send     []plannedLeg
	rejected []BookedLeg
}

func (s *Service) isSportsbook(source string) bool {
	return s.cfg.Profiles[source].Type == sportsbookType
}

func (s *Service) planSplit(legs []slipLeg, data slipData) []bookPlan {
	var plans []bookPlan
	position := map[string]int{}
	for _, leg := range legs {
		if !s.isSportsbook(leg.source) {
			continue
		}
		at, ok := position[leg.source]
		if !ok {
			at = len(plans)
			position[leg.source] = at
			plans = append(plans, bookPlan{book: leg.source, booker: s.bookers[leg.source]})
		}
		plans[at].add(leg, data)
	}
	return plans
}

func (s *Service) planSingle(book string, legs []slipLeg, data slipData) bookPlan {
	plan := bookPlan{book: book, booker: s.bookers[book]}
	for _, leg := range legs {
		plan.add(leg, data)
	}
	return plan
}

func (p *bookPlan) add(leg slipLeg, data slipData) {
	if p.booker == nil {
		p.rejected = append(p.rejected, BookedLeg{Index: leg.index, Reason: messageUnsupported})
		return
	}
	if leg.key.TeamID != "" {
		p.rejected = append(p.rejected, BookedLeg{Index: leg.index, Reason: reasonTeamMarket})
		return
	}
	link, ok := data.links[linkKey{source: p.book, eventID: leg.key.EventID}]
	if !ok {
		p.rejected = append(p.rejected, BookedLeg{Index: leg.index, Reason: reasonNotListed})
		return
	}
	selection := resolve.Selection{Line: leg.key.Line, Side: leg.side, Label: leg.label}
	if link.flipped {
		selection = resolve.Flip(leg.key.Code, selection)
	}
	p.send = append(p.send, plannedLeg{index: leg.index, leg: sources.BookLeg{
		SourceEventID: link.sourceEventID,
		Code:          leg.key.Code,
		Period:        leg.key.Period,
		Side:          string(selection.Side),
		Label:         selection.Label,
		Line:          selection.Line,
		PlayerName:    data.players[leg.key.PlayerID],
		Price:         data.odds(leg, p.book),
	}})
}

func (s *Service) execute(ctx context.Context, plans []bookPlan, stake float64) []BookResult {
	results := make([]BookResult, len(plans))
	var wg sync.WaitGroup
	for i, plan := range plans {
		wg.Go(func() { results[i] = s.run(ctx, plan, stake) })
	}
	wg.Wait()
	return results
}

func (s *Service) run(ctx context.Context, plan bookPlan, stake float64) BookResult {
	result := BookResult{Book: plan.book, Legs: slices.Clone(plan.rejected)}
	switch {
	case plan.booker == nil:
		return result.finish(StatusUnsupported, messageUnsupported, stake)
	case len(plan.send) == 0:
		return result.finish(StatusFailed, messageNothingToBook, stake)
	}
	slices.SortStableFunc(plan.send, func(a, b plannedLeg) int {
		return cmp.Compare(legFingerprint(a.leg), legFingerprint(b.leg))
	})
	sent := make([]sources.BookLeg, len(plan.send))
	for i, planned := range plan.send {
		sent[i] = planned.leg
	}
	outcome, err := s.bookIdempotent(ctx, plan.book, plan.booker, sent)
	if err != nil {
		status, message := classify(err)
		s.log.WarnContext(ctx, "booking failed", "book", plan.book, "legs", len(sent), "err", err)
		for _, planned := range plan.send {
			result.Legs = append(result.Legs, BookedLeg{Index: planned.index, Reason: message})
		}
		return result.finish(status, message, stake)
	}
	booked := outcome.booking
	result.Code, result.WebURL, result.AppURL, result.Cached = booked.Code, booked.WebURL, booked.AppURL, outcome.cached
	expires := booked.ExpiresAt.UTC()
	result.ExpiresAt = &expires
	result.Legs = append(result.Legs, mapBookedLegs(plan.send, booked.Legs)...)
	return result.finish("", "", stake)
}

func mapBookedLegs(sent []plannedLeg, returned []sources.BookedLeg) []BookedLeg {
	mapped := make([]BookedLeg, len(sent))
	for i, planned := range sent {
		mapped[i] = BookedLeg{Index: planned.index, Price: planned.leg.Price, Reason: reasonNotReturned}
	}
	for _, leg := range returned {
		if leg.Index < 0 || leg.Index >= len(sent) {
			continue
		}
		target := &mapped[leg.Index]
		target.OK, target.Reason = leg.OK, leg.Reason
		if leg.Price > 1 {
			target.Price = leg.Price
		}
	}
	return mapped
}

func classify(err error) (Status, string) {
	switch {
	case errors.Is(err, sources.ErrBookingBlocked), errors.Is(err, sources.ErrForbidden):
		return StatusBlocked, messageBlocked
	case errors.Is(err, sources.ErrBookingUnsupported):
		return StatusUnsupported, messageUnsupported
	default:
		return StatusFailed, messageFailed
	}
}

func (r BookResult) finish(status Status, message string, stake float64) BookResult {
	slices.SortFunc(r.Legs, func(a, b BookedLeg) int { return cmp.Compare(a.Index, b.Index) })
	odds, okLegs := 1.0, 0
	for _, leg := range r.Legs {
		if leg.OK {
			okLegs++
			if leg.Price > 1 {
				odds *= leg.Price
			}
		}
	}
	if status == "" {
		switch {
		case okLegs == len(r.Legs):
			status = StatusOK
		case okLegs == 0 && r.Code == "":
			status, message = StatusFailed, messageNothingToBook
		default:
			status = StatusPartial
		}
	}
	r.Status, r.Message = status, message
	if okLegs > 0 {
		r.Odds = round(odds, 4)
		r.Return = round(r.Odds*stake, 2)
	}
	return r
}

func manualLegs(legs []slipLeg, results []BookResult, data slipData) []ManualLeg {
	outcome := map[int]BookedLeg{}
	for _, result := range results {
		for _, leg := range result.Legs {
			if leg.OK || outcome[leg.Index].Reason == "" {
				outcome[leg.Index] = leg
			}
		}
	}
	manual := []ManualLeg{}
	for _, leg := range legs {
		booked, planned := outcome[leg.index]
		if booked.OK {
			continue
		}
		reason := booked.Reason
		if !planned {
			reason = reasonNoCodes
		}
		cell, _ := data.cell(leg, leg.source)
		manual = append(manual, ManualLeg{Index: leg.index, Source: leg.source, DeepLink: cell.DeepLink, Price: round(decimalOdds(cell), 4), Reason: reason})
	}
	return manual
}
