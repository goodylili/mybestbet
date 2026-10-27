package seed

import (
	"context"
	"errors"
	"strconv"

	"mybestbet/internal/stats/euroleague"
)

func EuroLeagueClubSourceID(leagueCode, clubCode string) string {
	return euroleague.Competitions[leagueCode] + ":" + clubCode
}

func EuroLeagueGameSourceID(seasonCode string, gameCode int) string {
	return seasonCode + ":" + strconv.Itoa(gameCode)
}

func (o *OtherSportsSeeder) seedEuroLeague(ctx context.Context, league League) error {
	now := o.now()
	games, err := o.euroleague.Games(ctx, league.Code, now)
	if err != nil {
		return err
	}
	clubIDs := map[string]string{}
	var errs []error
	for _, g := range games {
		for _, club := range []euroleague.Club{g.Home, g.Away} {
			if _, known := clubIDs[club.Code]; known {
				continue
			}
			id, err := o.store.UpsertSourceTeam(ctx, SourceTeam{
				Source: euroleague.SourceName, SourceID: EuroLeagueClubSourceID(league.Code, club.Code), LeagueCode: league.Code,
				Name: club.Name, Aliases: []string{club.ShortName}, LogoURL: club.LogoURL,
			})
			if err != nil {
				errs = append(errs, err)
				continue
			}
			clubIDs[club.Code] = id
		}
	}
	upcoming := 0
	season := euroleague.SeasonCode(euroleague.Competitions[league.Code], now)
	for _, g := range games {
		homeID, awayID := clubIDs[g.Home.Code], clubIDs[g.Away.Code]
		if homeID == "" || awayID == "" || !o.isInSeedWindow(g.Kickoff) {
			continue
		}
		upcoming++
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: euroleague.SourceName, SourceID: EuroLeagueGameSourceID(season, g.Code), LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: g.Kickoff, Status: euroLeagueStatus(g),
		})
		errs = append(errs, err)
	}
	errs = append(errs, o.seedEuroLeagueRosters(ctx, league, clubIDs))
	return o.logSeeded(euroleague.SourceName, league, len(clubIDs), upcoming, errors.Join(errs...))
}

func euroLeagueStatus(g euroleague.Game) EventStatus {
	if g.Played {
		return EventFinished
	}
	return EventScheduled
}

func (o *OtherSportsSeeder) seedEuroLeagueRosters(ctx context.Context, league League, clubIDs map[string]string) error {
	players, err := o.euroleague.Players(ctx, league.Code, o.now())
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range players {
		teamID, ok := clubIDs[p.ClubCode]
		if !ok {
			continue
		}
		_, err := o.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: euroleague.SourceName, SourceID: p.Code, TeamID: teamID, Name: p.Name, Position: NormalizePosition(league.Sport, p.Position)})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
