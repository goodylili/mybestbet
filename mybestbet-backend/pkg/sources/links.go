package sources

import (
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"mybestbet/pkg/canon"
)

type EventRef struct {
	SourceEventID  string
	SourceLeagueID string
	Sport          canon.Sport
	League         string
	Country        string
	LeagueName     string
	Home           string
	Away           string
}

func (r EventRef) LeagueTitle() string {
	if title := strings.TrimSpace(r.Country + " " + r.LeagueName); title != "" {
		return title
	}
	return r.League
}

func (r EventRef) Matchup(sep string) string {
	return Slug(r.Home, "-") + sep + Slug(r.Away, "-")
}

type EventLinker interface {
	EventURL(ref EventRef) string
}

func SlugWords(s string) []string {
	var words []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			word.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return words
}

func Slug(s, sep string) string {
	return strings.ToLower(strings.Join(SlugWords(s), sep))
}

func Origin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
