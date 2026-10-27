package flashscore

import (
	"fmt"
	"strings"
)

const (
	recordSeparator   = "~"
	fieldSeparator    = "¬"
	keyValueSeparator = "÷"
	embeddedFeedStart = "data: `"
	embeddedFeedEnd   = "`"
)

const (
	keySectionHeader = "ZA"
	keySectionPath   = "ZL"
	keyMatchID       = "AA"
	keyStartUnix     = "AD"
	keyStage         = "AB"
	keyStatusDetail  = "AC"
	keyHomeID        = "PX"
	keyAwayID        = "PY"
	keyHomeShortName = "AE"
	keyAwayShortName = "AF"
	keyHomeSlug      = "WU"
	keyAwaySlug      = "WV"
	keyStatPeriod    = "SE"
	keyStatName      = "SG"
	keyStatHome      = "SH"
	keyStatAway      = "SI"
)

type record map[string]string

func parseRecords(body string) []record {
	var records []record
	for _, raw := range strings.Split(body, recordSeparator) {
		rec := record{}
		for _, field := range strings.Split(raw, fieldSeparator) {
			if key, value, ok := strings.Cut(field, keyValueSeparator); ok {
				rec[key] = value
			}
		}
		if len(rec) > 0 {
			records = append(records, rec)
		}
	}
	return records
}

func embeddedFeed(page, feedName string) (string, error) {
	marker := "cjs.initialFeeds['" + feedName + "']"
	_, afterMarker, found := strings.Cut(page, marker)
	if !found {
		return "", fmt.Errorf("%s: %w", feedName, ErrFeedMissing)
	}
	_, afterStart, found := strings.Cut(afterMarker, embeddedFeedStart)
	if !found {
		return "", fmt.Errorf("%s: %w", feedName, ErrFeedMissing)
	}
	feed, _, found := strings.Cut(afterStart, embeddedFeedEnd)
	if !found {
		return "", fmt.Errorf("%s: %w", feedName, ErrFeedMissing)
	}
	return feed, nil
}
