package betking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"slices"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	defaultBettingURL     = "https://www.betking.com/api/sports/v1/bet"
	bookBetWebURL         = "https://m.betking.com/en-ng/sports/book-bet/"
	createCouponPath      = "/create/language/en"
	bookCouponPath        = "/Book/"
	bookedStatus          = 1
	transactionIDLength   = 20
	transactionIDSeparate = ","
	createAttempts        = 2
)

var errCouponRejected = errors.New("coupon rejected")

type couponSelection struct {
	SelectionID string `json:"SelectionId"`
}

type createCouponRequest struct {
	Stake      *float64          `json:"Stake"`
	Groupings  []json.RawMessage `json:"Groupings"`
	Selections []couponSelection `json:"Selections"`
}

type createCouponResponse struct {
	ResponseStatus int             `json:"ResponseStatus"`
	BetCoupon      json.RawMessage `json:"BetCoupon"`
}

type couponOdds struct {
	Odds []struct {
		SelectionID int64 `json:"SelectionId"`
	} `json:"Odds"`
}

type bookCouponResponse struct {
	ResponseStatus   int    `json:"ResponseStatus"`
	BookedCouponCode string `json:"BookedCouponCode"`
}

func (a *Adapter) Book(ctx context.Context, legs []sources.BookLeg) (sources.Booking, error) {
	resolved, err := sources.ResolveLegs(ctx, legs, a.eventItems, a.pickOdd)
	if err != nil {
		return sources.Booking{Legs: resolved.Legs}, fmt.Errorf("betking book: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	coupon, err := a.createCoupon(ctx, resolved.Refs)
	if err != nil {
		return sources.Booking{}, fmt.Errorf("betking create coupon: %w", sources.BookingFailure(err))
	}
	if err := rejectMissing(resolved, coupon); err != nil {
		return sources.Booking{Legs: resolved.Legs}, fmt.Errorf("betking create coupon: %w", err)
	}
	var booked bookCouponResponse
	target := a.bettingURL() + bookCouponPath + url.PathEscape(transactionID(resolved.Refs))
	if err := a.deps.PostJSON(ctx, target, coupon, &booked); err != nil {
		return sources.Booking{}, fmt.Errorf("betking book coupon: %w", sources.BookingFailure(err))
	}
	if booked.ResponseStatus != bookedStatus || booked.BookedCouponCode == "" {
		return sources.Booking{}, fmt.Errorf("betking book coupon: status %d: %w", booked.ResponseStatus, errCouponRejected)
	}
	code := booked.BookedCouponCode
	return sources.Booking{Code: code, WebURL: bookBetWebURL + url.PathEscape(code), Legs: resolved.Legs}, nil
}

func (a *Adapter) createCoupon(ctx context.Context, selectionIDs []int64) (json.RawMessage, error) {
	request := createCouponRequest{Selections: make([]couponSelection, 0, len(selectionIDs))}
	for _, id := range selectionIDs {
		request.Selections = append(request.Selections, couponSelection{SelectionID: strconv.FormatInt(id, 10)})
	}
	var err error
	for range createAttempts {
		var resp createCouponResponse
		if err = a.deps.PostJSON(ctx, a.bettingURL()+createCouponPath, request, &resp); err != nil {
			return nil, err
		}
		if len(resp.BetCoupon) > 0 && string(resp.BetCoupon) != "null" {
			return resp.BetCoupon, nil
		}
		err = fmt.Errorf("status %d: %w", resp.ResponseStatus, errCouponRejected)
	}
	return nil, err
}

func rejectMissing(resolved sources.ResolvedLegs[int64], coupon json.RawMessage) error {
	var parsed couponOdds
	if err := json.Unmarshal(coupon, &parsed); err != nil {
		return fmt.Errorf("decode coupon: %w", err)
	}
	onCoupon := make(map[int64]bool, len(parsed.Odds))
	for _, odd := range parsed.Odds {
		onCoupon[odd.SelectionID] = true
	}
	for i, id := range resolved.Refs {
		if !onCoupon[id] {
			resolved.Reject(i, sources.ErrLegSuspended.Error())
		}
	}
	if len(onCoupon) == 0 {
		return fmt.Errorf("empty coupon: %w", sources.ErrNothingBookable)
	}
	return nil
}

func transactionID(selectionIDs []int64) string {
	ids := make([]string, 0, len(selectionIDs))
	for _, id := range selectionIDs {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	slices.Sort(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, transactionIDSeparate)))
	return hex.EncodeToString(sum[:])[:transactionIDLength]
}

func (a *Adapter) pickOdd(items []matchItem, leg sources.BookLeg) (int64, float64, error) {
	return sources.PickLegOutcome(a.singleOdds(items), leg)
}

func (a *Adapter) singleOdds(items []matchItem) iter.Seq2[int64, []canon.RawMarket] {
	observedAt := a.deps.Clock()
	return func(yield func(int64, []canon.RawMarket) bool) {
		for _, item := range items {
			for _, collection := range item.OddsCollection {
				for _, odd := range collection.MatchOdds {
					single := collection
					single.MatchOdds = []matchOdd{odd}
					fragment := item
					fragment.OddsCollection = []oddsCollection{single}
					if !yield(odd.MatchOddsID, a.itemMarkets(fragment, observedAt)) {
						return
					}
				}
			}
		}
	}
}

func (a *Adapter) bettingURL() string {
	return a.deps.ExtraOr("bettingUrl", defaultBettingURL)
}
