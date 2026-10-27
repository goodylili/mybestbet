package betway

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	defaultBookingURL = "https://www.betway.com.ng/appsynapse/bet-api-sr02/v1/Betting/BookABet"
	defaultBrandID    = "f8a8d16a-d619-4b49-aa8c-f21211403c92"
	bookingURLExtra   = "bookingUrl"
	brandIDExtra      = "brandId"
	brandIDHeader     = "X-Brand-Id"
	webBookingURL     = "https://www.betway.com.ng/?bookingCode="
	minimumStake      = 100
	lineTolerance     = 1e-9

	reasonEventUnavailable = "event not available at betway"
	reasonNotOffered       = "selection not offered at betway"
	reasonSuspended        = "selection suspended at betway"

	playerMarketSeparator = "#"
)

var _ sources.Booker = (*Adapter)(nil)

var (
	errNothingBookable = errors.New("betway: no bookable legs")
	errNoBookingCode   = errors.New("betway: booking response has no code")
	errLoginRequired   = errors.New("betway creates booking codes only for logged-in accounts")
)

type bookedPick struct {
	index   int
	eventID int64
	outcome outcome
	price   float64
}

type eventLookup struct {
	eventID string
	resp    eventMarketsResponse
	markets []canon.RawMarket
	err     error
}

type bookingRequest struct {
	CultureCode string           `json:"cultureCode"`
	CountryCode string           `json:"countryCode"`
	IsSingleBet bool             `json:"isSingleBet"`
	IsBuildABet bool             `json:"isBuildABet"`
	Outcomes    []bookingOutcome `json:"outcomes"`
}

type bookingOutcome struct {
	OutcomeID string  `json:"outcomeId"`
	EventID   int64   `json:"eventId"`
	MarketID  string  `json:"marketId"`
	Payment   int     `json:"payment"`
	Value     float64 `json:"value"`
	Selected  bool    `json:"selected"`
}

type bookingResponse struct {
	BookingCode string `json:"bookingCode"`
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	booked := make([]sources.BookedLeg, len(legs))
	picks, err := a.resolvePicks(ctx, legs, booked)
	if err != nil {
		return sources.Booking{}, err
	}
	code, err := a.submitBooking(ctx, picks)
	if err != nil {
		return sources.Booking{}, err
	}
	return sources.Booking{
		Code:   code,
		WebURL: webBookingURL + url.QueryEscape(code),
		Legs:   booked,
	}, nil
}

func (a *Adapter) resolvePicks(ctx context.Context, legs []sources.BookLeg, booked []sources.BookedLeg) ([]bookedPick, error) {
	lookups := map[string]eventLookup{}
	var picks []bookedPick
	var fetchErrs []error
	for i, leg := range legs {
		booked[i] = sources.BookedLeg{Index: i}
		lookup, ok := lookups[leg.SourceEventID]
		if !ok {
			lookup = a.lookupEvent(ctx, leg.SourceEventID)
			if isBlocked(lookup.err) {
				return nil, fmt.Errorf("betway event %s: %w: %w", leg.SourceEventID, sources.ErrBookingBlocked, lookup.err)
			}
			if lookup.err != nil {
				fetchErrs = append(fetchErrs, lookup.err)
			}
			lookups[leg.SourceEventID] = lookup
		}
		pick, reason := pickFor(lookup, leg)
		if reason != "" {
			booked[i].Reason = reason
			continue
		}
		pick.index = i
		booked[i].OK = true
		booked[i].Price = pick.price
		picks = append(picks, pick)
	}
	if len(picks) > 0 {
		return picks, nil
	}
	return nil, errors.Join(append([]error{errNothingBookable}, fetchErrs...)...)
}

func (a *Adapter) lookupEvent(ctx context.Context, eventID string) eventLookup {
	resp, err := a.fetchEventMarkets(ctx, eventID)
	if err != nil {
		return eventLookup{err: fmt.Errorf("betway event %s: %w", eventID, err)}
	}
	return eventLookup{eventID: eventID, resp: resp, markets: a.eventMarkets(eventID, resp, a.deps.Clock())}
}

func pickFor(lookup eventLookup, leg sources.BookLeg) (bookedPick, string) {
	if lookup.err != nil {
		return bookedPick{}, reasonEventUnavailable
	}
	eventID, err := strconv.ParseInt(leg.SourceEventID, 10, 64)
	if err != nil {
		return bookedPick{}, reasonEventUnavailable
	}
	for _, m := range lookup.markets {
		if !matchesLeg(m, leg) {
			continue
		}
		for _, sel := range m.Selections {
			if string(sel.Side) != leg.Side || sel.Label != leg.Label {
				continue
			}
			o, ok := lookup.outcomeOf(m, sel)
			switch {
			case !ok:
				return bookedPick{}, reasonNotOffered
			case sel.Price.Suspended:
				return bookedPick{}, reasonSuspended
			}
			return bookedPick{eventID: eventID, outcome: o, price: sel.Price.Raw}, ""
		}
	}
	return bookedPick{}, reasonNotOffered
}

func (l eventLookup) outcomeOf(m canon.RawMarket, sel canon.Selection) (outcome, bool) {
	if m.Player != nil {
		originalMarketID, code, _ := strings.Cut(m.SourceMarketID, playerMarketSeparator)
		return l.outcomeWhere(func(o outcome) bool { return o.OutcomeID == originalMarketID+code })
	}
	specs := specsForEvent(l.eventID, l.resp.Outcomes)
	return l.outcomeWhere(func(o outcome) bool {
		if o.OriginalMarketID != m.SourceMarketID {
			return false
		}
		po := describeOutcome(l.eventID, o)
		native, ok := matchSelection(specs[po.typeCode], po)
		return ok && native.Side == sel.Side && native.Label == sel.Label
	})
}

func (l eventLookup) outcomeWhere(match func(outcome) bool) (outcome, bool) {
	i := slices.IndexFunc(l.resp.Outcomes, match)
	if i < 0 {
		return outcome{}, false
	}
	return l.resp.Outcomes[i], true
}

func matchesLeg(m canon.RawMarket, leg sources.BookLeg) bool {
	sameMarket := m.Code == leg.Code && m.Period == leg.Period && sameLine(m.Line, leg.Line)
	if !sameMarket {
		return false
	}
	if m.Player == nil {
		return leg.PlayerName == ""
	}
	return samePlayer(m.Player.Name, leg.PlayerName)
}

func sameLine(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) < lineTolerance
}

func samePlayer(sourceName, canonicalName string) bool {
	return canonicalName != "" && slices.Equal(nameTokens(sourceName), nameTokens(canonicalName))
}

func nameTokens(name string) []string {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == ' ' || r == ',' })
	slices.Sort(tokens)
	return tokens
}

func newBookingRequest(picks []bookedPick) bookingRequest {
	outcomes := make([]bookingOutcome, 0, len(picks))
	for _, p := range picks {
		outcomes = append(outcomes, bookingOutcome{
			OutcomeID: p.outcome.OutcomeID,
			EventID:   p.eventID,
			MarketID:  p.outcome.MarketID,
			Payment:   1,
			Value:     minimumStake,
			Selected:  true,
		})
	}
	return bookingRequest{CultureCode: cultureCode, CountryCode: countryCode, Outcomes: outcomes}
}

func (a *Adapter) submitBooking(ctx context.Context, picks []bookedPick) (string, error) {
	payload, err := json.Marshal(newBookingRequest(picks))
	if err != nil {
		return "", fmt.Errorf("encode betway booking: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.bookingURL(), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build betway booking request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(brandIDHeader, cmp.Or(a.deps.Config.Extra[brandIDExtra], defaultBrandID))
	body, err := a.deps.Do(req)
	var status *sources.StatusError
	switch {
	case errors.As(err, &status) && status.Status == http.StatusUnauthorized:
		return "", fmt.Errorf("betway booking: %w: %w", sources.ErrBookingUnsupported, errLoginRequired)
	case isBlocked(err):
		return "", fmt.Errorf("betway booking: %w: %w", sources.ErrBookingBlocked, err)
	case err != nil:
		return "", fmt.Errorf("betway booking: %w", err)
	}
	return decodeBooking(body)
}

func decodeBooking(body []byte) (string, error) {
	var resp bookingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode betway booking: %w", err)
	}
	if resp.BookingCode == "" {
		return "", errNoBookingCode
	}
	return resp.BookingCode, nil
}

func (a *Adapter) bookingURL() string {
	return cmp.Or(a.deps.Config.Extra[bookingURLExtra], defaultBookingURL)
}

func isBlocked(err error) bool {
	var status *sources.StatusError
	isLegalBlock := errors.As(err, &status) && status.Status == http.StatusUnavailableForLegalReasons
	isForbidden := errors.As(err, &status) && status.Status == http.StatusForbidden
	return isLegalBlock || isForbidden
}
