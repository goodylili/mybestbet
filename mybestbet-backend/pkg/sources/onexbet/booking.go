package onexbet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	json "github.com/goccy/go-json"
	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	saveCouponPath         = "/service-api/LiveBet/Open/SaveCoupon"
	couponCodeQuery        = "coupon-code"
	prematchEventKind      = 3
	singleCouponVid        = 0
	accumulatorCouponVid   = 1
	acceptOddsChanges      = 1
	couponLanguage         = "en"
	lineTolerance          = 1e-9
	saveCouponAttempts     = 3
	callFailedErrorCode    = 161627
	saveCouponRetryDelay   = 500 * time.Millisecond
	reasonEventNotListed   = "event not listed at this book"
	reasonSelectionMissing = "selection not offered or suspended at this book"
)

var (
	errNoBookableLegs = errors.New("no bookable legs")
	errEmptyCoupon    = errors.New("save coupon returned no code")
	errCallFailed     = errors.New("save coupon call failed")
)

type couponEvent struct {
	GameID       int64   `json:"GameId"`
	Type         int     `json:"Type"`
	Coef         float64 `json:"Coef"`
	Param        float64 `json:"Param"`
	PlayerID     int64   `json:"PlayerId"`
	Kind         int     `json:"Kind"`
	InstrumentID int     `json:"InstrumentId"`
	Seconds      int     `json:"Seconds"`
	Price        float64 `json:"Price"`
	Expired      int     `json:"Expired"`
}

type saveCouponRequest struct {
	Events  []couponEvent `json:"Events"`
	Summ    float64       `json:"Summ"`
	Vid     int           `json:"Vid"`
	Partner int           `json:"partner"`
	Lng     string        `json:"Lng"`
	UserID  int64         `json:"UserId"`
	CheckCf int           `json:"CheckCf"`
	NotWait bool          `json:"notWait"`
}

type legMatch struct {
	event  couponEvent
	start  int64
	listed bool
	found  bool
}

func (m legMatch) reason() string {
	if m.listed {
		return reasonSelectionMissing
	}
	return reasonEventNotListed
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	matches, lookupErr := a.matchLegs(ctx, legs)
	booked := make([]sources.BookedLeg, len(legs))
	events := make([]couponEvent, 0, len(legs))
	var firstStart int64
	for i, m := range matches {
		booked[i] = sources.BookedLeg{Index: i, Reason: m.reason()}
		if !m.found {
			continue
		}
		booked[i] = sources.BookedLeg{Index: i, Price: m.event.Coef, OK: true}
		events = append(events, m.event)
		if firstStart == 0 || (m.start > 0 && m.start < firstStart) {
			firstStart = m.start
		}
	}
	if len(events) == 0 {
		return sources.Booking{}, nothingBookable(lookupErr)
	}
	code, err := a.saveCoupon(ctx, events)
	if err != nil {
		return sources.Booking{}, err
	}
	booking := sources.Booking{Code: code, WebURL: a.shareURL(code), Legs: booked}
	if firstStart > 0 {
		booking.ExpiresAt = time.Unix(firstStart, 0).UTC()
	}
	return booking, nil
}

func nothingBookable(lookupErr error) error {
	switch {
	case errors.Is(lookupErr, sources.ErrForbidden):
		return fmt.Errorf("%w: %w", sources.ErrBookingBlocked, lookupErr)
	case lookupErr != nil:
		return fmt.Errorf("%w: %w", errNoBookableLegs, lookupErr)
	}
	return errNoBookableLegs
}

func (a *Adapter) matchLegs(ctx context.Context, legs []sources.BookLeg) ([]legMatch, error) {
	byEvent := map[string][]int{}
	var order []string
	for i, leg := range legs {
		if _, seen := byEvent[leg.SourceEventID]; !seen {
			order = append(order, leg.SourceEventID)
		}
		byEvent[leg.SourceEventID] = append(byEvent[leg.SourceEventID], i)
	}
	matches := make([]legMatch, len(legs))
	eventErrs := make([]error, len(order))
	observedAt := a.deps.Clock()
	var group errgroup.Group
	group.SetLimit(concurrentEventFetch)
	for n, eventID := range order {
		indexes := byEvent[eventID]
		group.Go(func() error {
			main, _, err := a.walkEvent(ctx, a.bookingBase, eventID, observedAt, func(o offer) {
				for _, i := range indexes {
					if !matches[i].found && o.matches(legs[i]) {
						matches[i] = legMatch{event: o.couponEvent(), found: true}
					}
				}
			})
			if err != nil {
				eventErrs[n] = fmt.Errorf("book event %s: %w", eventID, err)
				return nil
			}
			for _, i := range indexes {
				matches[i].start, matches[i].listed = main.StartUnix, true
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return matches, err
	}
	return matches, errors.Join(eventErrs...)
}

func (o offer) couponEvent() couponEvent {
	event := couponEvent{GameID: o.game.ID, Type: o.bet.Type, Coef: o.bet.Odds, Kind: prematchEventKind}
	if o.bet.Param != nil {
		event.Param = *o.bet.Param
	}
	if o.bet.Player != nil {
		event.PlayerID = o.bet.Player.ID
	}
	return event
}

func (o offer) matches(leg sources.BookLeg) bool {
	sel := o.placed.selection
	samePick := string(sel.Side) == leg.Side && sel.Label == leg.Label
	if o.where.period != leg.Period || !samePick || !samePlayer(o.placed.player, leg.PlayerName) {
		return false
	}
	if o.rule.code == leg.Code {
		return sameLine(o.placed.line, leg.Line)
	}
	market := accumulatedMarket{group: o.group, raw: canon.RawMarket{Code: o.rule.code, Line: o.placed.line, Period: o.where.period}}
	code, ok := market.levelHandicapEquivalent()
	return ok && code == leg.Code && leg.Line == nil
}

func sameLine(offered, wanted *float64) bool {
	if offered == nil || wanted == nil {
		return offered == nil && wanted == nil
	}
	return math.Abs(*offered-*wanted) < lineTolerance
}

func samePlayer(offered *canon.SourcePlayer, wanted string) bool {
	if offered == nil || wanted == "" {
		return offered == nil && wanted == ""
	}
	return samePersonName(offered.Name, wanted)
}

func samePersonName(a, b string) bool {
	wordsA, wordsB := nameWords(a), nameWords(b)
	return coversName(wordsA, wordsB) || coversName(wordsB, wordsA)
}

func coversName(short, long []string) bool {
	fullWords := 0
	for _, word := range short {
		matched := false
		for _, other := range long {
			if word == other || (len([]rune(word)) == 1 && strings.HasPrefix(other, word)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
		if len([]rune(word)) > 1 {
			fullWords++
		}
	}
	return fullWords > 0
}

func nameWords(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
}

func (a *Adapter) saveCoupon(ctx context.Context, events []couponEvent) (string, error) {
	partner, err := strconv.Atoi(a.partner)
	if err != nil {
		return "", fmt.Errorf("save coupon partner %q: %w", a.partner, err)
	}
	vid := singleCouponVid
	if len(events) > 1 {
		vid = accumulatorCouponVid
	}
	payload, err := json.Marshal(saveCouponRequest{
		Events:  events,
		Vid:     vid,
		Partner: partner,
		Lng:     couponLanguage,
		CheckCf: acceptOddsChanges,
		NotWait: true,
	})
	if err != nil {
		return "", fmt.Errorf("encode save coupon: %w", err)
	}
	for attempt := 1; ; attempt++ {
		code, err := a.postCoupon(ctx, payload)
		if !errors.Is(err, errCallFailed) || attempt == saveCouponAttempts {
			return code, err
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("save coupon retry: %w", ctx.Err())
		case <-time.After(a.retryDelay):
		}
	}
}

func (a *Adapter) postCoupon(ctx context.Context, payload []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.bookingBase+saveCouponPath, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build save coupon request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := a.deps.Do(req)
	if errors.Is(err, sources.ErrForbidden) {
		return "", fmt.Errorf("save coupon: %w: %w", sources.ErrBookingBlocked, err)
	}
	if err != nil {
		return "", fmt.Errorf("save coupon: %w", err)
	}
	var resp response[string]
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode save coupon: %w", err)
	}
	if resp.ErrorCode == callFailedErrorCode {
		return "", fmt.Errorf("save coupon: %w: %w", errCallFailed, resp.err())
	}
	if err := resp.err(); err != nil {
		return "", fmt.Errorf("save coupon: %w", err)
	}
	if resp.Value == "" {
		return "", errEmptyCoupon
	}
	return resp.Value, nil
}

func (a *Adapter) shareURL(code string) string {
	return a.webURL + "/?" + couponCodeQuery + "=" + url.QueryEscape(code)
}
