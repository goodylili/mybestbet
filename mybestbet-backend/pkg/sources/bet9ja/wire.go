package bet9ja

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const responseOK = "OK"

type response[T any] struct {
	Result string `json:"R"`
	Data   T      `json:"D"`
}

func (r response[T]) err() error {
	if r.Result == responseOK {
		return nil
	}
	return fmt.Errorf("feed result %q", r.Result)
}

type sportsTree struct {
	Sports map[string]treeSport `json:"PAL"`
}

type treeSport struct {
	Name       string                  `json:"S_DESC"`
	Categories map[string]treeCategory `json:"SG"`
}

type treeCategory struct {
	Name   string               `json:"SG_DESC"`
	Groups map[string]treeGroup `json:"G"`
}

type treeGroup struct {
	Name string `json:"G_DESC"`
	Date string `json:"D"`
}

type groupPage struct {
	Events       []event       `json:"E"`
	MarketGroups []marketGroup `json:"GMK"`
	MainGroup    string        `json:"RGM"`
}

type marketGroup struct {
	ID string `json:"GMID"`
}

type event struct {
	ID          int64           `json:"ID"`
	Description string          `json:"DS"`
	StartDate   string          `json:"STARTDATE"`
	StartUTC    string          `json:"STARTDATEUTC"`
	GroupID     int64           `json:"GID"`
	GroupName   string          `json:"GN"`
	Odds        map[string]odds `json:"O"`
}

const teamSeparator = " - "

func (e event) teams() (home, away string, ok bool) {
	home, away, ok = strings.Cut(e.Description, teamSeparator)
	return strings.TrimSpace(home), strings.TrimSpace(away), ok && home != "" && away != ""
}

var startDateLayouts = []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}

func (e event) kickoff() (time.Time, error) {
	for _, value := range []string{e.StartUTC, e.StartDate} {
		for _, layout := range startDateLayouts {
			if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
				return t.UTC(), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("event %d: unparseable start date %q", e.ID, e.StartDate)
}

type odds float64

func (o *odds) UnmarshalJSON(data []byte) error {
	unquoted := bytes.Trim(bytes.TrimSpace(data), `"`)
	if len(unquoted) == 0 || string(unquoted) == "null" {
		*o = 0
		return nil
	}
	value, err := strconv.ParseFloat(string(unquoted), 64)
	if err != nil {
		return fmt.Errorf("odds %s: %w", data, err)
	}
	*o = odds(value)
	return nil
}
