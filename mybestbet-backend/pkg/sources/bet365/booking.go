package bet365

import (
	"context"
	"errors"
	"fmt"

	"mybestbet/pkg/sources"
)

var _ sources.Booker = (*Adapter)(nil)

var errBookingGeoBlocked = errors.New("bet365: www.bet365.com answers 403 from cloudflare (page title 'Attention Required! | Cloudflare') from this network, and bet365 publishes no booking-code or share-betslip endpoint")

func (a *Adapter) Book(context.Context, []sources.BookLeg) (sources.Booking, error) {
	return sources.Booking{}, fmt.Errorf("%w: %w", sources.ErrBookingBlocked, errBookingGeoBlocked)
}
