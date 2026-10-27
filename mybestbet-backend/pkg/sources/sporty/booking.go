package sporty

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"path"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const sharePath = "/orders/share"

var shareURLFormats = map[string]string{
	SportyBet:   "%s/%s/?shareCode=%s&c=%[2]s",
	FootballCom: "%s/%s/m?shareCode=%s",
}

type shareSelection struct {
	EventID   string `json:"eventId"`
	MarketID  string `json:"marketId"`
	Specifier string `json:"specifier,omitempty"`
	OutcomeID string `json:"outcomeId"`
}

type shareRequest struct {
	Selections []shareSelection `json:"selections"`
}

type shareData struct {
	ShareCode           string           `json:"shareCode"`
	Deadline            int64            `json:"deadline"`
	UnavailableOutcomes []shareSelection `json:"unavailableOutcomes"`
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	resolved, err := sources.ResolveLegs(ctx, legs, a.event, a.pickSelection)
	if err != nil {
		return sources.Booking{Legs: resolved.Legs}, fmt.Errorf("%s book: %w", a.name, err)
	}
	body, err := json.Marshal(shareRequest{Selections: resolved.Refs})
	if err != nil {
		return sources.Booking{}, fmt.Errorf("%s encode share: %w", a.name, err)
	}
	var resp envelope[shareData]
	if err := a.fetchJSON(ctx, http.MethodPost, sharePath, body, &resp); err != nil {
		return sources.Booking{}, fmt.Errorf("%s share: %w", a.name, sources.BookingFailure(err))
	}
	if err := resp.err(); err != nil {
		return sources.Booking{}, fmt.Errorf("%s share: %w", a.name, err)
	}
	if resp.Data.ShareCode == "" {
		return sources.Booking{}, fmt.Errorf("%s share: empty share code", a.name)
	}
	rejectUnavailable(resolved, resp.Data.UnavailableOutcomes)
	booking := sources.Booking{Code: resp.Data.ShareCode, WebURL: a.shareURL(resp.Data.ShareCode), Legs: resolved.Legs}
	if resp.Data.Deadline > 0 {
		booking.ExpiresAt = time.UnixMilli(resp.Data.Deadline).UTC()
	}
	return booking, nil
}

func rejectUnavailable(resolved sources.ResolvedLegs[shareSelection], unavailable []shareSelection) {
	gone := make(map[shareSelection]bool, len(unavailable))
	for _, s := range unavailable {
		gone[s] = true
	}
	for i, ref := range resolved.Refs {
		if gone[ref] {
			resolved.Reject(i, sources.ErrLegSuspended.Error())
		}
	}
}

func (a *Adapter) pickSelection(ev event, leg sources.BookLeg) (shareSelection, float64, error) {
	return sources.PickLegOutcome(a.singleOutcomes(ev), leg)
}

func (a *Adapter) singleOutcomes(ev event) iter.Seq2[shareSelection, []canon.RawMarket] {
	observedAt := a.deps.Clock()
	return func(yield func(shareSelection, []canon.RawMarket) bool) {
		for _, m := range ev.Markets {
			for _, o := range m.Outcomes {
				single := m
				single.Outcomes = []outcome{o}
				fragment := ev
				fragment.Markets = []market{single}
				ref := shareSelection{EventID: ev.EventID, MarketID: m.ID, Specifier: m.Specifier, OutcomeID: o.ID}
				if !yield(ref, a.rawMarkets(fragment, observedAt)) {
					return
				}
			}
		}
	}
}

func (a *Adapter) shareURL(code string) string {
	base, err := url.Parse(a.deps.Config.BaseURL)
	if err != nil {
		return ""
	}
	origin := base.Scheme + "://" + base.Host
	return fmt.Sprintf(shareURLFormats[a.name], origin, path.Base(base.Path), url.QueryEscape(code))
}
