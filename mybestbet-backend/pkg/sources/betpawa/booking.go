package betpawa

import (
	"context"
	"fmt"
	"iter"
	"net/url"
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	bookingAPIVersion = "/v3"
	listAPIVersion    = "/v4"
	bookingPath       = "/booking-number"
	singleSelection   = "SINGLE"
)

type bookingRequest struct {
	Selections bookingSelections `json:"selections"`
}

type bookingSelections struct {
	Selections []bookingEntry `json:"selections"`
}

type bookingEntry struct {
	Type       string  `json:"type"`
	Selections []int64 `json:"selections"`
}

type bookingResponse struct {
	Code string `json:"code"`
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	resolved, err := sources.ResolveLegs(ctx, legs, a.event, a.pickPrice)
	if err != nil {
		return sources.Booking{Legs: resolved.Legs}, fmt.Errorf("betpawa book: %w", err)
	}
	request := bookingRequest{}
	for _, priceID := range resolved.Refs {
		request.Selections.Selections = append(request.Selections.Selections, bookingEntry{Type: singleSelection, Selections: []int64{priceID}})
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var resp bookingResponse
	if err := a.deps.PostJSON(ctx, a.bookingURL(), request, &resp); err != nil {
		return sources.Booking{}, fmt.Errorf("betpawa booking number: %w", sources.BookingFailure(err))
	}
	if resp.Code == "" {
		return sources.Booking{}, fmt.Errorf("betpawa booking number: empty code")
	}
	return sources.Booking{Code: resp.Code, WebURL: a.bookingWebURL(resp.Code), Legs: resolved.Legs}, nil
}

func (a *Adapter) pickPrice(ev event, leg sources.BookLeg) (int64, float64, error) {
	return sources.PickLegOutcome(a.singlePrices(ev), leg)
}

func (a *Adapter) singlePrices(ev event) iter.Seq2[int64, []canon.RawMarket] {
	observedAt := a.deps.Clock()
	return func(yield func(int64, []canon.RawMarket) bool) {
		for _, m := range ev.Markets {
			for _, r := range m.Row {
				for _, p := range r.Prices {
					priceID, err := strconv.ParseInt(p.ID, 10, 64)
					if err != nil {
						continue
					}
					fragment := ev
					fragment.Markets = []market{{MarketType: m.MarketType, Row: []row{{Specifier: r.Specifier, Prices: []price{p}}}}}
					if !yield(priceID, a.eventMarkets(fragment, observedAt)) {
						return
					}
				}
			}
		}
	}
}

func (a *Adapter) bookingURL() string {
	return strings.TrimSuffix(a.deps.Config.BaseURL, listAPIVersion) + bookingAPIVersion + bookingPath
}

func (a *Adapter) bookingWebURL(code string) string {
	base, err := url.Parse(a.deps.Config.BaseURL)
	if err != nil {
		return ""
	}
	return base.Scheme + "://" + base.Host + "/?bookingCode=" + url.QueryEscape(code)
}
