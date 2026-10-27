package server

import (
	"errors"
	"net/http"

	json "github.com/goccy/go-json"

	"mybestbet/internal/review"
)

const (
	defaultReviewLimit   = 100
	maxReviewLimit       = 500
	maxDecisionBodyBytes = 16 << 10
)

func (s *Server) listReview(w http.ResponseWriter, r *http.Request) {
	status, err := enumQuery(r, "status", review.StatusPending, review.StatusLinked, review.StatusCreated, review.StatusIgnored)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	if status == "" {
		status = review.StatusPending
	}
	limit, err := boundedIntQuery(r, "limit", defaultReviewLimit, 1, maxReviewLimit)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	items, err := s.review.List(r.Context(), status, limit)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	for i := range items {
		items[i].Candidates = emptyIfNil(items[i].Candidates)
	}
	writeJSON(w, http.StatusOK, listOf(items))
}

func (s *Server) decideReview(w http.ResponseWriter, r *http.Request) {
	var decision review.Decision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDecisionBodyBytes)).Decode(&decision); err != nil {
		s.writeFailure(w, r, invalidInput("body", "must be a json review decision"))
		return
	}
	item, err := s.review.Decide(r.Context(), decision)
	if err != nil {
		s.writeFailure(w, r, reviewFailure(err))
		return
	}
	item.Candidates = emptyIfNil(item.Candidates)
	writeJSON(w, http.StatusOK, item)
}

func reviewFailure(err error) error {
	switch {
	case errors.Is(err, review.ErrInvalidDecision):
		return invalidInput("decision", err.Error())
	case errors.Is(err, review.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, review.ErrAlreadyDecided):
		return ErrConflict
	}
	return err
}
