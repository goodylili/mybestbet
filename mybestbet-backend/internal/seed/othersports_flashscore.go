package seed

import (
	"context"
	"errors"

	"mybestbet/internal/stats/flashscore"
)

const flashscoreSeedOldestDayOffset = -1

var eventStatusByFlashscore = map[flashscore.MatchStatus]EventStatus{
	flashscore.StatusScheduled: EventScheduled,
	flashscore.StatusLive:      EventLive,
	flashscore.StatusFinished:  EventFinished,
	flashscore.StatusRetired:   EventFinished,
	flashscore.StatusWalkover:  EventCancelled,
	flashscore.StatusCancelled: EventCancelled,
}

type flashscoreTennisWindow struct {
	byLeague map[string][]flashscore.Match
	err      error
}

func StatusFromFlashscore(status flashscore.MatchStatus) EventStatus {
	if eventStatus, ok := eventStatusByFlashscore[status]; ok {
		return eventStatus
	}
	return EventScheduled
}

func FlashscoreCompetitor(leagueCode string, p flashscore.Player) SourceTeam {
	return SourceTeam{Source: flashscore.SourceName, SourceID: p.ID, LeagueCode: leagueCode, Name: p.Name, Aliases: []string{p.ShortName}}
}

func (o *OtherSportsSeeder) seedFlashscoreTennisLeague(ctx context.Context, league League) error {
	if !hasKey(flashscore.Tournaments, league.Code) {
		return o.seedTennisLeague(ctx, league)
	}
	window := o.flashscoreTennisWindow(ctx)
	if window.err != nil {
		o.log.Warn("flashscore tennis window failed, seeding from espn", "source", flashscore.SourceName, "league", league.Code, "err", window.err)
	}
	matches := window.byLeague[league.Code]
	if window.err != nil || len(matches) == 0 {
		return o.seedTennisLeague(ctx, league)
	}
	competitors := map[string]string{}
	var errs []error
	for _, m := range matches {
		homeID, homeErr := o.flashscoreCompetitorID(ctx, league, m.Home, competitors)
		awayID, awayErr := o.flashscoreCompetitorID(ctx, league, m.Away, competitors)
		if homeErr != nil || awayErr != nil {
			errs = append(errs, homeErr, awayErr)
			continue
		}
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: flashscore.SourceName, SourceID: m.ID, LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: m.Kickoff, Status: StatusFromFlashscore(m.Status),
		})
		errs = append(errs, err)
	}
	return o.logSeeded(flashscore.SourceName, league, len(competitors), len(matches), errors.Join(errs...))
}

func (o *OtherSportsSeeder) flashscoreTennisWindow(ctx context.Context) *flashscoreTennisWindow {
	if o.flashscoreTennis != nil {
		return o.flashscoreTennis
	}
	window := &flashscoreTennisWindow{byLeague: map[string][]flashscore.Match{}}
	seen := map[string]bool{}
	for offset := flashscoreSeedOldestDayOffset; offset <= flashscore.NewestDayOffset; offset++ {
		matches, err := o.flashscore.DayMatches(ctx, offset)
		if err != nil {
			window.err = err
			break
		}
		for _, m := range matches {
			if !seen[m.ID] && o.isInSeedWindow(m.Kickoff) {
				seen[m.ID] = true
				window.byLeague[m.LeagueCode] = append(window.byLeague[m.LeagueCode], m)
			}
		}
	}
	o.flashscoreTennis = window
	return window
}

func (o *OtherSportsSeeder) flashscoreCompetitorID(ctx context.Context, league League, p flashscore.Player, competitors map[string]string) (string, error) {
	if id, ok := competitors[p.ID]; ok {
		return id, nil
	}
	teamID, _, err := o.store.UpsertCompetitor(ctx, FlashscoreCompetitor(league.Code, p))
	if err != nil {
		return "", err
	}
	competitors[p.ID] = teamID
	return teamID, nil
}
