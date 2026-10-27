package booking

import (
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"

	"mybestbet/pkg/canon"
)

const (
	MaxLegs        = 30
	maxLabelLength = 64
	maxStake       = 1e9
	defaultStake   = 1
)

var validSides = map[canon.Side]bool{
	"": true, canon.SideHome: true, canon.SideDraw: true, canon.SideAway: true,
	canon.SideOver: true, canon.SideUnder: true, canon.SideYes: true, canon.SideNo: true,
}

type slipLeg struct {
	index  int
	source string
	key    canon.MarketKey
	keyStr string
	side   canon.Side
	label  string
}

func (l slipLeg) identity() string {
	return l.keyStr + "\x00" + l.source + "\x00" + string(l.side) + "\x00" + l.label
}

func (s *Service) parseLegs(legs []Leg) ([]slipLeg, error) {
	if len(legs) == 0 {
		return nil, invalidInput("legs", "at least one leg is required")
	}
	if len(legs) > MaxLegs {
		return nil, invalidInput("legs", "at most "+strconv.Itoa(MaxLegs)+" legs are allowed")
	}
	parsed := make([]slipLeg, len(legs))
	seen := make(map[string]bool, len(legs))
	for i, leg := range legs {
		slip, err := s.parseLeg(i, leg)
		if err != nil {
			return nil, err
		}
		if seen[slip.identity()] {
			return nil, invalidInput(legField(i, "marketKey"), "duplicate leg")
		}
		seen[slip.identity()] = true
		parsed[i] = slip
	}
	return parsed, nil
}

func (s *Service) parseLeg(i int, leg Leg) (slipLeg, error) {
	if _, err := uuid.Parse(leg.EventID); err != nil {
		return slipLeg{}, invalidInput(legField(i, "eventId"), "must be a uuid")
	}
	key, err := canon.ParseMarketKey(leg.MarketKey)
	if err != nil {
		return slipLeg{}, invalidInput(legField(i, "marketKey"), "must be a market key")
	}
	if key.EventID != leg.EventID {
		return slipLeg{}, invalidInput(legField(i, "marketKey"), "must belong to eventId")
	}
	if _, known := canon.SpecOf(key.Code); !known {
		return slipLeg{}, invalidInput(legField(i, "marketKey"), "unknown market code")
	}
	if _, known := s.cfg.Sources[leg.Source]; !known {
		return slipLeg{}, invalidInput(legField(i, "source"), "unknown source")
	}
	side := canon.Side(leg.Side)
	if !validSides[side] {
		return slipLeg{}, invalidInput(legField(i, "side"), "unknown side")
	}
	if !utf8.ValidString(leg.Label) || len(leg.Label) > maxLabelLength {
		return slipLeg{}, invalidInput(legField(i, "label"), "must be at most 64 bytes of utf-8")
	}
	if side == "" && leg.Label == "" {
		return slipLeg{}, invalidInput(legField(i, "side"), "side or label is required")
	}
	return slipLeg{index: i, source: leg.Source, key: key, keyStr: leg.MarketKey, side: side, label: leg.Label}, nil
}

func legField(i int, name string) string { return fmt.Sprintf("legs[%d].%s", i, name) }

func normalizeStake(stake float64) (float64, error) {
	switch {
	case stake == 0:
		return defaultStake, nil
	case math.IsNaN(stake) || stake < 0 || stake > maxStake:
		return 0, invalidInput("stake", "must be between 0 and 1e9")
	}
	return stake, nil
}
