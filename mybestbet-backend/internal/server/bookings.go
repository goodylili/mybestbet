package server

import (
	"errors"
	"net/http"

	"mybestbet/internal/auth"
	"mybestbet/internal/booking"
)

const maxBookingBodyBytes = 64 << 10

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) {
	body, principal, ok := s.bookingBody(w, r)
	if !ok {
		return
	}
	result, err := s.bookings.Book(r.Context(), principal.UserID, body)
	if err != nil {
		s.writeFailure(w, r, bookingFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) compareBookings(w http.ResponseWriter, r *http.Request) {
	body, principal, ok := s.bookingBody(w, r)
	if !ok {
		return
	}
	comparison, err := s.bookings.Compare(r.Context(), principal.UserID, body)
	if err != nil {
		s.writeFailure(w, r, bookingFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, comparison)
}

func (s *Server) bookingBody(w http.ResponseWriter, r *http.Request) (booking.Request, auth.Principal, bool) {
	w.Header().Set("Cache-Control", "no-store")
	var body booking.Request
	principal, _ := auth.PrincipalFrom(r.Context())
	if s.bookings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "booking is not configured")
		return body, principal, false
	}
	if err := decodeBody(w, r, maxBookingBodyBytes, &body); err != nil {
		s.writeFailure(w, r, err)
		return body, principal, false
	}
	return body, principal, true
}

func bookingFailure(err error) error {
	var input *booking.InputError
	if errors.As(err, &input) {
		return invalidInput(input.Field, input.Reason)
	}
	var limited *booking.RateLimitError
	if errors.As(err, &limited) {
		return &auth.RateLimitError{RetryAfter: limited.RetryAfter}
	}
	return err
}
