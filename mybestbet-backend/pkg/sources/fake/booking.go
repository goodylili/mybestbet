package fake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	bookingPath   = "/booking"
	fallbackPrice = 2.0
	reasonNoEvent = "event not found"
)

var errNothingBookable = errors.New("fake: no bookable legs")

type bookingRequest struct {
	Selections []sources.BookLeg `json:"selections"`
}

type bookingResponse struct {
	Code string `json:"code"`
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	if a.deps.Config.BaseURL == "" {
		return sources.Booking{}, sources.ErrBookingUnsupported
	}
	booked := make([]sources.BookedLeg, len(legs))
	var selections []sources.BookLeg
	for i, leg := range legs {
		if !a.knows(leg.SourceEventID) {
			booked[i] = sources.BookedLeg{Index: i, Reason: reasonNoEvent}
			continue
		}
		price := leg.Price
		if price <= 1 {
			price = fallbackPrice
		}
		booked[i] = sources.BookedLeg{Index: i, Price: price, OK: true}
		selections = append(selections, leg)
	}
	if len(selections) == 0 {
		return sources.Booking{}, errNothingBookable
	}
	code, err := a.share(ctx, selections)
	if err != nil {
		return sources.Booking{}, err
	}
	return sources.Booking{
		Code:   code,
		WebURL: a.deps.Config.BaseURL + "/?shareCode=" + url.QueryEscape(code),
		AppURL: "fake://booking/" + url.PathEscape(code),
		Legs:   booked,
	}, nil
}

func (a *Adapter) knows(sourceEventID string) bool {
	return slices.ContainsFunc(a.events, func(e canon.SourceEvent) bool { return e.SourceID == sourceEventID })
}

func (a *Adapter) share(ctx context.Context, selections []sources.BookLeg) (string, error) {
	payload, err := json.Marshal(bookingRequest{Selections: selections})
	if err != nil {
		return "", fmt.Errorf("encode fake booking: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.deps.Config.BaseURL+bookingPath, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build fake booking request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := a.deps.Do(req)
	var status *sources.StatusError
	isLegalBlock := errors.As(err, &status) && status.Status == http.StatusUnavailableForLegalReasons
	if isLegalBlock || errors.Is(err, sources.ErrForbidden) {
		return "", fmt.Errorf("fake booking: %w", sources.ErrBookingBlocked)
	}
	if err != nil {
		return "", err
	}
	var decoded bookingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode fake booking: %w", err)
	}
	return decoded.Code, nil
}
