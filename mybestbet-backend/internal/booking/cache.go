package booking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/pkg/sources"
)

const (
	defaultCodeLifetime = 30 * time.Minute
	maxCodeLifetime     = 24 * time.Hour
	cacheKeyPrefix      = "booking:code:v1:"
)

var errEmptyCode = errors.New("booking: book returned no code")

type bookOutcome struct {
	booking sources.Booking
	cached  bool
}

func legFingerprint(leg sources.BookLeg) string {
	line := ""
	if leg.Line != nil {
		line = strconv.FormatFloat(*leg.Line, 'f', -1, 64)
	}
	return strings.Join([]string{leg.SourceEventID, string(leg.Code), leg.Period, leg.Side, leg.Label, line, leg.PlayerName}, "\x1f")
}

func cacheKey(book string, legs []sources.BookLeg) string {
	digest := sha256.New()
	digest.Write([]byte(book))
	for _, leg := range legs {
		digest.Write([]byte{0})
		digest.Write([]byte(legFingerprint(leg)))
	}
	return cacheKeyPrefix + hex.EncodeToString(digest.Sum(nil))
}

func (s *Service) bookIdempotent(ctx context.Context, book string, booker sources.Booker, legs []sources.BookLeg) (bookOutcome, error) {
	key := cacheKey(book, legs)
	if found, ok := s.cachedBooking(ctx, key); ok {
		return bookOutcome{booking: found, cached: true}, nil
	}
	detached := context.WithoutCancel(ctx)
	value, err, shared := s.flight.Do(key, func() (any, error) {
		if found, ok := s.cachedBooking(detached, key); ok {
			return bookOutcome{booking: found, cached: true}, nil
		}
		callCtx, cancel := context.WithTimeout(detached, s.callTimeout)
		defer cancel()
		booked, err := booker.Book(callCtx, legs)
		if err != nil {
			return nil, fmt.Errorf("book %s: %w", book, err)
		}
		if booked.Code == "" {
			return nil, fmt.Errorf("book %s: %w", book, errEmptyCode)
		}
		booked = s.withExpiry(booked)
		s.storeBooking(detached, key, booked)
		return bookOutcome{booking: booked}, nil
	})
	if err != nil {
		return bookOutcome{}, err
	}
	outcome, _ := value.(bookOutcome)
	outcome.cached = outcome.cached || shared
	return outcome, nil
}

func (s *Service) withExpiry(booked sources.Booking) sources.Booking {
	now := s.now()
	lifetime := booked.ExpiresAt.Sub(now)
	if booked.ExpiresAt.IsZero() || lifetime <= 0 {
		booked.ExpiresAt = now.Add(defaultCodeLifetime)
	} else if lifetime > maxCodeLifetime {
		booked.ExpiresAt = now.Add(maxCodeLifetime)
	}
	return booked
}

func (s *Service) cachedBooking(ctx context.Context, key string) (sources.Booking, bool) {
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return sources.Booking{}, false
	}
	if err != nil {
		s.log.WarnContext(ctx, "read cached booking", "err", err)
		return sources.Booking{}, false
	}
	var found sources.Booking
	if err := json.Unmarshal(raw, &found); err != nil {
		s.log.WarnContext(ctx, "decode cached booking", "err", err)
		return sources.Booking{}, false
	}
	return found, found.ExpiresAt.After(s.now())
}

func (s *Service) storeBooking(ctx context.Context, key string, booked sources.Booking) {
	encoded, err := json.Marshal(booked)
	if err != nil {
		s.log.WarnContext(ctx, "encode booking", "err", err)
		return
	}
	if err := s.rdb.Set(ctx, key, encoded, booked.ExpiresAt.Sub(s.now())).Err(); err != nil {
		s.log.WarnContext(ctx, "cache booking", "err", err)
	}
}
