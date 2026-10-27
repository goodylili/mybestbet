package betano

import (
	"context"
	"errors"
	"fmt"

	"mybestbet/pkg/sources"
)

var _ sources.Booker = (*Adapter)(nil)

var errBookingGeoBlocked = errors.New("betano: www.betano.ng answers 403 from cloudflare (page title 'Betano Splash Screen') to every request from this network, including GET /api/sharebookingcode/getbetslip; the share flow (POST /api/betslip/v3/AddPartsAndSingleLineAmount, then POST /api/sharebookingcode/betslip) cannot be called or verified")

func (a *Adapter) Book(context.Context, []sources.BookLeg) (sources.Booking, error) {
	return sources.Booking{}, fmt.Errorf("%w: %w", sources.ErrBookingBlocked, errBookingGeoBlocked)
}
