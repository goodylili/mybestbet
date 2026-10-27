package bo3

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/sources"
)

const (
	SourceName       = "bo3"
	DefaultBaseURL   = "https://api.bo3.gg/api/v1"
	tiersExtra       = "tiers"
	defaultTiers     = "s,a"
	matchesPageLimit = 100
	maxMatchPages    = 20
	statusFinished   = "finished"
	statusLive       = "current"
	statusDefaultWin = "defwin"
)

var ErrUnknownDiscipline = errors.New("bo3: no discipline for league code")

var Disciplines = map[string]int{"CS2": 1, "VALORANT": 2, "LOL": 3}

type Client struct {
	deps    sources.Deps
	baseURL string
	tiers   string
}

type Team struct {
	ID      string
	Slug    string
	Name    string
	LogoURL string
}

type Match struct {
	ID     int64
	Slug   string
	Start  time.Time
	Status string
	Tier   string
	Team1  Team
	Team2  Team
	Maps   [2]int
}

func (m Match) IsFinished() bool { return m.Status == statusFinished }

func (m Match) IsLive() bool { return m.Status == statusLive }

func (m Match) IsDecided() bool { return m.IsFinished() || m.Status == statusDefaultWin }

type Player struct {
	ID   string
	Name string
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	tiers := deps.Config.Extra[tiersExtra]
	if tiers == "" {
		tiers = defaultTiers
	}
	return &Client{deps: deps, baseURL: baseURL, tiers: tiers}
}

func Supports(leagueCode string) bool {
	_, ok := Disciplines[leagueCode]
	return ok
}

func (c *Client) Matches(ctx context.Context, leagueCode string, from, to time.Time) ([]Match, error) {
	discipline, ok := Disciplines[leagueCode]
	if !ok {
		return nil, fmt.Errorf("%s: %w", leagueCode, ErrUnknownDiscipline)
	}
	query := url.Values{
		"filter[matches.discipline_id][eq]": {strconv.Itoa(discipline)},
		"filter[matches.start_date][gt]":    {from.UTC().Format(time.RFC3339)},
		"filter[matches.start_date][lt]":    {to.UTC().Format(time.RFC3339)},
		"filter[matches.tier][in]":          {c.tiers},
		"page[limit]":                       {strconv.Itoa(matchesPageLimit)},
		"sort":                              {"start_date"},
		"with":                              {"teams"},
	}
	var matches []Match
	for page := range maxMatchPages {
		query.Set("page[offset]", strconv.Itoa(page*matchesPageLimit))
		var resp matchPageWire
		if err := c.deps.GetJSON(ctx, c.baseURL+"/matches?"+query.Encode(), &resp); err != nil {
			return nil, fmt.Errorf("bo3 %s matches: %w", leagueCode, err)
		}
		for _, m := range resp.Results {
			if match, ok := matchFrom(m); ok {
				matches = append(matches, match)
			}
		}
		if len(resp.Results) < matchesPageLimit || (page+1)*matchesPageLimit >= resp.Total.Count {
			break
		}
	}
	return matches, nil
}

func (c *Client) Match(ctx context.Context, slug string) (Match, error) {
	var resp matchWire
	if err := c.deps.GetJSON(ctx, c.baseURL+"/matches/"+url.PathEscape(slug)+"?with=teams", &resp); err != nil {
		return Match{}, fmt.Errorf("bo3 match %s: %w", slug, err)
	}
	match, ok := matchFrom(resp)
	if !ok {
		return Match{}, fmt.Errorf("bo3 match %s: %w", slug, sources.ErrNotFound)
	}
	return match, nil
}

func (c *Client) Roster(ctx context.Context, teamSlug string) ([]Player, error) {
	var resp teamRosterWire
	if err := c.deps.GetJSON(ctx, c.baseURL+"/teams/"+url.PathEscape(teamSlug)+"?with=players", &resp); err != nil {
		return nil, fmt.Errorf("bo3 team %s roster: %w", teamSlug, err)
	}
	roster := make([]Player, 0, len(resp.Players))
	for _, p := range resp.Players {
		if !p.IsCoach && p.Nickname != "" {
			roster = append(roster, playerFrom(p))
		}
	}
	return roster, nil
}

func matchFrom(m matchWire) (Match, bool) {
	if m.Team1 == nil || m.Team2 == nil || m.Slug == "" {
		return Match{}, false
	}
	return Match{
		ID: m.ID, Slug: m.Slug, Start: m.StartDate.UTC(), Status: m.Status, Tier: m.Tier,
		Team1: teamFrom(*m.Team1), Team2: teamFrom(*m.Team2), Maps: [2]int{m.Team1Maps, m.Team2Maps},
	}, true
}

func teamFrom(t teamWire) Team {
	return Team{ID: idText(t.ID), Slug: t.Slug, Name: t.Name, LogoURL: t.ImageURL}
}

func playerFrom(p playerWire) Player {
	return Player{ID: idText(p.ID), Name: p.Nickname}
}

func idText(id int64) string { return strconv.FormatInt(id, 10) }
