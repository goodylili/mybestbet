package esports

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"mybestbet/pkg/sources"
)

const (
	OpenDotaSource         = "opendota"
	DefaultOpenDotaBaseURL = "https://api.opendota.com/api"
)

type SeriesType int

const (
	BestOfOne SeriesType = iota
	BestOfThree
	BestOfFive
	BestOfTwo
)

type OpenDota struct {
	deps    sources.Deps
	baseURL string
}

type DotaTeam struct {
	ID      string
	Name    string
	Tag     string
	LogoURL string
}

type DotaPlayer struct {
	AccountID string
	Name      string
}

type ProGame struct {
	MatchID      int64
	Start        time.Time
	Radiant      DotaTeam
	Dire         DotaTeam
	SeriesID     int64
	SeriesType   SeriesType
	League       string
	Live         bool
	RadiantWin   bool
	RadiantKills int
	DireKills    int
}

type GamePlayer struct {
	DotaPlayer
	IsRadiant bool
	Kills     int
}

type GameDetail struct {
	MatchID int64
	Start   time.Time
	Radiant DotaTeam
	Dire    DotaTeam
	Players []GamePlayer
}

func NewOpenDota(deps sources.Deps) *OpenDota {
	return &OpenDota{deps: deps, baseURL: baseURLOr(deps, DefaultOpenDotaBaseURL)}
}

func (o *OpenDota) ProGames(ctx context.Context) ([]ProGame, error) {
	var resp []proMatchWire
	if err := o.deps.GetJSON(ctx, o.baseURL+"/proMatches", &resp); err != nil {
		return nil, fmt.Errorf("opendota pro matches: %w", err)
	}
	games := make([]ProGame, 0, len(resp))
	for _, m := range resp {
		games = append(games, ProGame{
			MatchID: m.MatchID, Start: time.Unix(m.StartTime, 0).UTC(), SeriesID: m.SeriesID, SeriesType: SeriesType(m.SeriesType), League: m.LeagueName,
			Radiant:    DotaTeam{ID: idText(m.RadiantTeamID), Name: m.RadiantName},
			Dire:       DotaTeam{ID: idText(m.DireTeamID), Name: m.DireName},
			RadiantWin: m.RadiantWin, RadiantKills: m.RadiantScore, DireKills: m.DireScore,
		})
	}
	return games, nil
}

func (o *OpenDota) LiveGames(ctx context.Context) ([]ProGame, error) {
	var resp []liveGameWire
	if err := o.deps.GetJSON(ctx, o.baseURL+"/live", &resp); err != nil {
		return nil, fmt.Errorf("opendota live games: %w", err)
	}
	var games []ProGame
	for _, g := range resp {
		if g.LeagueID == 0 || g.DeactivateTime != 0 || g.RadiantTeamID == 0 || g.DireTeamID == 0 {
			continue
		}
		games = append(games, ProGame{
			MatchID: g.MatchID, Start: time.Unix(g.ActivateTime, 0).UTC(), SeriesID: g.SeriesID, Live: true,
			Radiant:      DotaTeam{ID: idText(g.RadiantTeamID), Name: g.RadiantName},
			Dire:         DotaTeam{ID: idText(g.DireTeamID), Name: g.DireName},
			RadiantKills: g.RadiantScore, DireKills: g.DireScore,
		})
	}
	return games, nil
}

func (o *OpenDota) Game(ctx context.Context, matchID int64) (GameDetail, error) {
	var resp matchWire
	if err := o.deps.GetJSON(ctx, fmt.Sprintf("%s/matches/%d", o.baseURL, matchID), &resp); err != nil {
		return GameDetail{}, fmt.Errorf("opendota match %d: %w", matchID, err)
	}
	game := GameDetail{MatchID: resp.MatchID, Start: time.Unix(resp.StartTime, 0).UTC(), Radiant: dotaTeamFrom(resp.RadiantTeam), Dire: dotaTeamFrom(resp.DireTeam)}
	for _, p := range resp.Players {
		if p.AccountID == 0 {
			continue
		}
		name := p.Name
		if name == "" {
			name = p.Personaname
		}
		game.Players = append(game.Players, GamePlayer{DotaPlayer: DotaPlayer{AccountID: idText(p.AccountID), Name: name}, IsRadiant: p.IsRadiant, Kills: p.Kills})
	}
	return game, nil
}

func (o *OpenDota) Teams(ctx context.Context) ([]DotaTeam, error) {
	var resp []teamWire
	if err := o.deps.GetJSON(ctx, o.baseURL+"/teams", &resp); err != nil {
		return nil, fmt.Errorf("opendota teams: %w", err)
	}
	teams := make([]DotaTeam, 0, len(resp))
	for _, t := range resp {
		teams = append(teams, dotaTeamFrom(t))
	}
	return teams, nil
}

func (o *OpenDota) CurrentRoster(ctx context.Context, teamID string) ([]DotaPlayer, error) {
	var resp []teamPlayerWire
	if err := o.deps.GetJSON(ctx, o.baseURL+"/teams/"+url.PathEscape(teamID)+"/players", &resp); err != nil {
		return nil, fmt.Errorf("opendota team %s players: %w", teamID, err)
	}
	var roster []DotaPlayer
	for _, p := range resp {
		if p.IsCurrentTeamMember && p.Name != "" {
			roster = append(roster, DotaPlayer{AccountID: idText(p.AccountID), Name: p.Name})
		}
	}
	return roster, nil
}

func (t SeriesType) IsDecided(homeWins, awayWins int) bool {
	if t == BestOfTwo {
		return homeWins+awayWins >= 2
	}
	return max(homeWins, awayWins) > int(t)
}

func dotaTeamFrom(t teamWire) DotaTeam {
	return DotaTeam{ID: idText(t.TeamID), Name: t.Name, Tag: t.Tag, LogoURL: t.LogoURL}
}

func idText(id int64) string { return strconv.FormatInt(id, 10) }
