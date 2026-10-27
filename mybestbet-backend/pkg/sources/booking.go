package sources

import (
	"context"
	"errors"
	"time"

	"mybestbet/pkg/canon"
)

type BookLeg struct {
	SourceEventID string
	Code          canon.MarketCode
	Period        string
	Side          string
	Label         string
	Line          *float64
	PlayerName    string
	Price         float64
}

type BookedLeg struct {
	Index  int
	Price  float64
	OK     bool
	Reason string
}

type Booking struct {
	Code      string
	WebURL    string
	AppURL    string
	Legs      []BookedLeg
	ExpiresAt time.Time
}

type Booker interface {
	Book(ctx context.Context, legs []BookLeg) (Booking, error)
}

var (
	ErrBookingUnsupported = errors.New("booking unsupported")
	ErrBookingBlocked     = errors.New("booking blocked from this region")
)
