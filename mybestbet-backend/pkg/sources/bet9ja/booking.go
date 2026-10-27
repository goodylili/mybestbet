package bet9ja

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/sources"
)

const (
	defaultBookingURL = "https://apigw.bet9ja.com/sportsbook/placebet/BookABetV2?source=desktop"
	bookingURLExtra   = "bookingUrl"
	webBookingURL     = "https://sports.bet9ja.com/?bookABetCode="
	bookingOrigin     = "https://sports.bet9ja.com"
	betslipField      = "BETSLIP"
	bookedStatus      = 1
	lineTolerance     = 1e-9

	reasonEventUnavailable = "event not available at bet9ja"
	reasonNotOffered       = "selection not offered at bet9ja"
	reasonPlayerProp       = "player props are not bookable at bet9ja"
	reasonSameEvent        = "bet9ja books one selection per event"
)

var _ sources.Booker = (*Adapter)(nil)

var (
	errNothingBookable = errors.New("bet9ja: no bookable legs")
	errNoBookingCode   = errors.New("bet9ja: booking response has no code")
)

type bookedPick struct {
	index   int
	eventID string
	key     string
	price   float64
}

func (p bookedPick) id() string { return p.eventID + "$" + p.key }

type eventLookup struct {
	markets []nativeMarket
	err     error
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	booked := make([]sources.BookedLeg, len(legs))
	picks, err := a.resolvePicks(ctx, legs, booked)
	if err != nil {
		return sources.Booking{}, err
	}
	code, prices, err := a.submitBooking(ctx, picks)
	if err != nil {
		return sources.Booking{}, err
	}
	for _, p := range picks {
		booked[p.index].Price = cmp.Or(prices[p.id()], p.price)
	}
	return sources.Booking{
		Code:   code,
		WebURL: webBookingURL + url.QueryEscape(code),
		Legs:   booked,
	}, nil
}

func (a *Adapter) resolvePicks(ctx context.Context, legs []sources.BookLeg, booked []sources.BookedLeg) ([]bookedPick, error) {
	lookups := map[string]eventLookup{}
	bookedEvents := map[string]bool{}
	var picks []bookedPick
	var fetchErrs []error
	for i, leg := range legs {
		booked[i] = sources.BookedLeg{Index: i}
		if leg.PlayerName != "" {
			booked[i].Reason = reasonPlayerProp
			continue
		}
		lookup, ok := lookups[leg.SourceEventID]
		if !ok {
			lookup = a.lookupEvent(ctx, leg.SourceEventID)
			if isBlocked(lookup.err) {
				return nil, fmt.Errorf("bet9ja event %s: %w: %w", leg.SourceEventID, sources.ErrBookingBlocked, lookup.err)
			}
			if lookup.err != nil {
				fetchErrs = append(fetchErrs, lookup.err)
			}
			lookups[leg.SourceEventID] = lookup
		}
		pick, reason := pickFor(lookup, leg)
		pick.index = i
		switch {
		case reason != "":
			booked[i].Reason = reason
		case bookedEvents[pick.eventID]:
			booked[i].Reason = reasonSameEvent
		default:
			bookedEvents[pick.eventID] = true
			booked[i].OK = true
			booked[i].Price = pick.price
			picks = append(picks, pick)
		}
	}
	if len(picks) > 0 {
		return picks, nil
	}
	return nil, errors.Join(append([]error{errNothingBookable}, fetchErrs...)...)
}

func (a *Adapter) lookupEvent(ctx context.Context, eventID string) eventLookup {
	ev, err := a.fetchEvent(ctx, eventID)
	if err != nil {
		return eventLookup{err: fmt.Errorf("bet9ja event %s: %w", eventID, err)}
	}
	return eventLookup{markets: a.nativeMarkets(ev, a.deps.Clock())}
}

func pickFor(lookup eventLookup, leg sources.BookLeg) (bookedPick, string) {
	if lookup.err != nil {
		return bookedPick{}, reasonEventUnavailable
	}
	for _, m := range lookup.markets {
		if !m.matches(leg) {
			continue
		}
		for i, sel := range m.raw.Selections {
			if string(sel.Side) == leg.Side && sel.Label == leg.Label {
				return bookedPick{eventID: leg.SourceEventID, key: m.keys[i], price: sel.Price.Raw}, ""
			}
		}
	}
	return bookedPick{}, reasonNotOffered
}

func (m nativeMarket) matches(leg sources.BookLeg) bool {
	return m.raw.Code == leg.Code && m.raw.Period == leg.Period && sameLine(m.raw.Line, leg.Line)
}

func sameLine(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) < lineTolerance
}

type betslip struct {
	Bets        []betslipBet            `json:"BETS"`
	Events      map[string]betslipEvent `json:"EVS"`
	Impersonize int                     `json:"IMPERSONIZE"`
}

type betslipBet struct {
	SlipType  int                `json:"BSTYPE"`
	Tab       int                `json:"TAB"`
	NumLines  int                `json:"NUMLINES"`
	Comb      int                `json:"COMB"`
	Type      int                `json:"TYPE"`
	Stake     float64            `json:"STAKE"`
	PotWinMin float64            `json:"POTWINMIN"`
	PotWinMax float64            `json:"POTWINMAX"`
	BonusMin  float64            `json:"BONUSMIN"`
	BonusMax  float64            `json:"BONUSMAX"`
	OddMin    float64            `json:"ODDMIN"`
	OddMax    float64            `json:"ODDMAX"`
	Odds      map[string]float64 `json:"ODDS"`
	Fixed     map[string]bool    `json:"FIXED"`
}

type betslipEvent struct {
	EventID string `json:"eventId"`
	Market  string `json:"market"`
	Sign    string `json:"sign"`
	SID     string `json:"sid"`
	ID      string `json:"id"`
}

type bookingResponse struct {
	Status int `json:"status"`
	Error  struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Data []struct {
		Code  string `json:"RIS"`
		Lines map[string]struct {
			Odds float64 `json:"oddValue"`
		} `json:"BETLINES"`
	} `json:"data"`
}

func newBetslip(picks []bookedPick) betslip {
	odds := make(map[string]float64, len(picks))
	events := make(map[string]betslipEvent, len(picks))
	total := 1.0
	for _, p := range picks {
		parsed, _ := parseOddsKey(p.key)
		odds[p.id()] = p.price
		events[p.id()] = betslipEvent{EventID: p.eventID, Market: parsed.market, Sign: parsed.outcome, SID: p.key, ID: p.id()}
		total *= p.price
	}
	total = math.Round(total*100) / 100
	return betslip{
		Bets: []betslipBet{{
			NumLines: 1,
			Comb:     1,
			Type:     len(picks),
			OddMin:   total,
			OddMax:   total,
			Odds:     odds,
			Fixed:    map[string]bool{},
		}},
		Events: events,
	}
}

func (a *Adapter) submitBooking(ctx context.Context, picks []bookedPick) (string, map[string]float64, error) {
	slip, err := json.Marshal(newBetslip(picks))
	if err != nil {
		return "", nil, fmt.Errorf("encode bet9ja betslip: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	form := url.Values{betslipField: {string(slip)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.bookingURL(), strings.NewReader(form))
	if err != nil {
		return "", nil, fmt.Errorf("build bet9ja booking request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", bookingOrigin)
	body, err := a.deps.Do(req)
	if isBlocked(err) {
		return "", nil, fmt.Errorf("bet9ja booking: %w: %w", sources.ErrBookingBlocked, err)
	}
	if err != nil {
		return "", nil, fmt.Errorf("bet9ja booking: %w", err)
	}
	return decodeBooking(body)
}

func decodeBooking(body []byte) (string, map[string]float64, error) {
	var resp bookingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", nil, fmt.Errorf("decode bet9ja booking: %w", err)
	}
	if resp.Status != bookedStatus {
		return "", nil, fmt.Errorf("bet9ja booking rejected: status %d, error %d %q", resp.Status, resp.Error.Code, resp.Error.Message)
	}
	if len(resp.Data) == 0 || resp.Data[0].Code == "" {
		return "", nil, errNoBookingCode
	}
	prices := make(map[string]float64, len(resp.Data[0].Lines))
	for id, line := range resp.Data[0].Lines {
		prices[id] = line.Odds
	}
	return resp.Data[0].Code, prices, nil
}

func (a *Adapter) bookingURL() string {
	return cmp.Or(a.deps.Config.Extra[bookingURLExtra], defaultBookingURL)
}

func isBlocked(err error) bool {
	var status *sources.StatusError
	isLegalBlock := errors.As(err, &status) && status.Status == http.StatusUnavailableForLegalReasons
	return isLegalBlock || errors.Is(err, sources.ErrForbidden)
}
