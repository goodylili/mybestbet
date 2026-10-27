package esports

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"mybestbet/pkg/sources"
)

const (
	LiquipediaSource         = "liquipedia"
	DefaultLiquipediaBaseURL = "https://liquipedia.net"
	matchesPage              = "/api.php?action=parse&page=Liquipedia:Matches&format=json&prop=text"
	upcomingMatchSelector    = `[data-toggle-area-content="1"] .match-info`
	missingPageSuffix        = " (page does not exist)"
)

var ErrUnknownGame = errors.New("esports: no liquipedia wiki for league code")

var Wikis = map[string]string{"DOTA2": "dota2", "CS2": "counterstrike", "VALORANT": "valorant", "LOL": "leagueoflegends"}

var (
	matchPageID = regexp.MustCompile(`Match:ID_([A-Za-z0-9_]+)`)
	bestOf      = regexp.MustCompile(`\(Bo(\d+)\)`)
)

type Liquipedia struct {
	deps    sources.Deps
	baseURL string
}

type Team struct {
	Name      string
	ShortName string
	LogoURL   string
}

type Match struct {
	ID         string
	LeagueCode string
	Kickoff    time.Time
	Tournament string
	BestOf     int
	Team1      Team
	Team2      Team
}

func NewLiquipedia(deps sources.Deps) *Liquipedia {
	return &Liquipedia{deps: deps, baseURL: baseURLOr(deps, DefaultLiquipediaBaseURL)}
}

func baseURLOr(deps sources.Deps, fallback string) string {
	if base := strings.TrimRight(deps.Config.BaseURL, "/"); base != "" {
		return base
	}
	return fallback
}

func (l *Liquipedia) UpcomingMatches(ctx context.Context, leagueCode string) ([]Match, error) {
	wiki, ok := Wikis[leagueCode]
	if !ok {
		return nil, fmt.Errorf("%s: %w", leagueCode, ErrUnknownGame)
	}
	var resp parseResponse
	if err := l.deps.GetJSON(ctx, l.baseURL+"/"+wiki+matchesPage, &resp); err != nil {
		return nil, fmt.Errorf("liquipedia %s matches: %w", wiki, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.Parse.Text.HTML))
	if err != nil {
		return nil, fmt.Errorf("liquipedia %s matches html: %w", wiki, err)
	}
	var matches []Match
	doc.Find(upcomingMatchSelector).Each(func(_ int, s *goquery.Selection) {
		if m, ok := matchFrom(leagueCode, l.baseURL, s); ok {
			matches = append(matches, m)
		}
	})
	return matches, nil
}

func matchFrom(leagueCode, baseURL string, s *goquery.Selection) (Match, bool) {
	timestamp, err := strconv.ParseInt(s.Find(".timer-object").AttrOr("data-timestamp", ""), 10, 64)
	if err != nil {
		return Match{}, false
	}
	opponents := s.Find(".match-info-header-opponent")
	if opponents.Length() != 2 {
		return Match{}, false
	}
	team1, ok1 := teamFrom(opponents.Eq(0), baseURL)
	team2, ok2 := teamFrom(opponents.Eq(1), baseURL)
	if !ok1 || !ok2 {
		return Match{}, false
	}
	m := Match{
		LeagueCode: leagueCode,
		Kickoff:    time.Unix(timestamp, 0).UTC(),
		Tournament: strings.TrimSpace(s.Find(".match-info-tournament-name").Text()),
		Team1:      team1,
		Team2:      team2,
	}
	if found := bestOf.FindStringSubmatch(s.Find(".match-info-header-scoreholder-lower").Text()); found != nil {
		m.BestOf, _ = strconv.Atoi(found[1])
	}
	m.ID = matchID(s, m)
	return m, true
}

func teamFrom(opponent *goquery.Selection, baseURL string) (Team, bool) {
	link := opponent.Find(".name a").First()
	if link.Length() == 0 {
		return Team{}, false
	}
	short := strings.TrimSpace(link.Text())
	full := strings.TrimSuffix(link.AttrOr("title", short), missingPageSuffix)
	return Team{Name: full, ShortName: short, LogoURL: teamIconURL(opponent, baseURL)}, full != ""
}

func teamIconURL(opponent *goquery.Selection, baseURL string) string {
	icon := opponent.Find(".team-template-image-icon img").First()
	src := icon.AttrOr("src", "")
	if candidates := strings.Split(icon.AttrOr("srcset", ""), ","); len(candidates) > 0 {
		if largest := strings.Fields(candidates[len(candidates)-1]); len(largest) > 0 {
			src = largest[0]
		}
	}
	if !strings.HasPrefix(src, "/") || strings.HasPrefix(src, "//") {
		return ""
	}
	return baseURL + src
}

func matchID(s *goquery.Selection, m Match) string {
	html, _ := s.Html()
	if found := matchPageID.FindStringSubmatch(html); found != nil {
		return found[1]
	}
	return fmt.Sprintf("%d:%s:%s", m.Kickoff.Unix(), m.Team1.Name, m.Team2.Name)
}
