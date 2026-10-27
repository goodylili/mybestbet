package server

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"mybestbet/pkg/canon"
)

func uuidPathValue(r *http.Request, name string) (string, error) {
	raw := r.PathValue(name)
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", invalidInput(name, "must be a uuid")
	}
	return id.String(), nil
}

func boundedIntQuery(r *http.Request, name string, fallback, low, high int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < low || n > high {
		return 0, invalidInput(name, "must be an integer between "+strconv.Itoa(low)+" and "+strconv.Itoa(high))
	}
	return n, nil
}

func floatQuery(r *http.Request, name string) (float64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, invalidInput(name, "must be a number")
	}
	return v, nil
}

func timeQuery(r *http.Request, name string, fallback time.Time) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, invalidInput(name, "must be an RFC3339 timestamp")
	}
	return t, nil
}

func enumQuery(r *http.Request, name string, allowed ...string) (string, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return "", nil
	}
	for _, v := range allowed {
		if raw == v {
			return raw, nil
		}
	}
	return "", invalidInput(name, "unsupported value")
}

func timeWindowQuery(r *http.Request, defaultFrom time.Time, defaultSpan, maxSpan time.Duration) (time.Time, time.Time, error) {
	from, err := timeQuery(r, "from", defaultFrom)
	if err != nil {
		return from, from, err
	}
	to, err := timeQuery(r, "to", from.Add(defaultSpan))
	if err != nil {
		return from, to, err
	}
	if !to.After(from) || to.Sub(from) > maxSpan {
		return from, to, invalidInput("to", fmt.Sprintf("must be after from and within %d days", int(maxSpan.Hours()/24)))
	}
	return from, to, nil
}

func locationQuery(r *http.Request, name string) (*time.Location, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(raw)
	if err != nil || raw == "Local" {
		return nil, invalidInput(name, "must be an IANA time zone")
	}
	return loc, nil
}

var (
	bestOutcomeShapes = []canon.MarketShape{canon.ShapeThreeWay, canon.ShapeTwoWay, canon.ShapeHandicap, canon.ShapeOverUnder, canon.ShapeYesNo}
	lineShapes        = []canon.MarketShape{canon.ShapeHandicap, canon.ShapeOverUnder}
)

func marketSelectorQuery(r *http.Request) (marketSelector, error) {
	period, err := enumQuery(r, "period", slices.Collect(maps.Keys(periodOrder))...)
	if err != nil {
		return marketSelector{}, err
	}
	selector := marketSelector{code: canon.MarketCode(r.URL.Query().Get("market")), period: period}
	if r.URL.Query().Get("line") != "" {
		line, err := floatQuery(r, "line")
		if err != nil {
			return marketSelector{}, err
		}
		selector.line = &line
	}
	if selector.code == "" {
		if selector.line != nil {
			return marketSelector{}, invalidInput("line", "requires market")
		}
		return selector, nil
	}
	spec, known := canon.SpecOf(selector.code)
	if !known || spec.Kind != canon.KindMatch || !slices.Contains(bestOutcomeShapes, spec.Shape) {
		return marketSelector{}, invalidInput("market", "must be a match market with home/draw/away, over/under or yes/no selections")
	}
	if slices.Contains(lineShapes, spec.Shape) != (selector.line != nil) {
		return marketSelector{}, invalidInput("line", "required for handicap and over/under markets, not allowed otherwise")
	}
	return selector, nil
}
