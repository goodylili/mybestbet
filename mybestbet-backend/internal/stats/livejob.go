package stats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/internal/seed"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/pkg/canon"
)

const (
	liveScoreInterval  = 30 * time.Second
	liveScoreLookback  = 4 * time.Hour
	liveScoreLookahead = 15 * time.Minute
	mapPeriodPrefix    = "M"
	oneDay             = 24 * time.Hour
	unlinkedFootball   = "unlinked_football"
	overdueFallback    = 6 * time.Hour
	overdueQuietFor    = 30 * time.Minute
)

var overdueAfterKickoff = map[canon.Sport]time.Duration{
	canon.SportFootball:         3 * time.Hour,
	canon.SportBasketball:       3*time.Hour + 30*time.Minute,
	canon.SportAmericanFootball: 4*time.Hour + 30*time.Minute,
	canon.SportTennis:           6 * time.Hour,
	canon.SportEsports:          6 * time.Hour,
}

var (
	errUnknownLiveFeed = errors.New("stats: no live score feed")
	liveScoreSources   = []string{
		fotmob.SourceName, espn.SourceName, flashscore.SourceName, bo3.SourceName, esports.LiquipediaSource, euroleague.SourceName, acb.SourceName,
	}
	usScheduleDateOffsets    = []time.Duration{-4 * time.Hour, -5 * time.Hour}
	flashscoreLiveDayOffsets = []int{-1, 0}
)

type LiveScoreCollector struct {
	fotmob     *fotmob.Client
	espn       *espn.Client
	flashscore *flashscore.Client
	bo3        *bo3.Client
	opendota   *esports.OpenDota
	euroleague *euroleague.Client
	acb        *acb.Client
	store      *Store
	redis      *goredis.Client
	sportOf    func(leagueCode string) canon.Sport
	log        *slog.Logger
	now        func() time.Time
}

type LiveScoreCollectorDeps struct {
	FotMob     *fotmob.Client
	ESPN       *espn.Client
	Flashscore *flashscore.Client
	Bo3        *bo3.Client
	OpenDota   *esports.OpenDota
	EuroLeague *euroleague.Client
	ACB        *acb.Client
	Store      *Store
	Redis      *goredis.Client
	SportOf    func(leagueCode string) canon.Sport
	Log        *slog.Logger
	Now        func() time.Time
}

type LiveSportCount struct {
	Polled  int
	Scored  int
	Updated int
}

type LiveRound map[canon.Sport]*LiveSportCount

type liveScore struct {
	event    SourcedEvent
	score    canon.EventScore
	finished bool
}

func NewLiveScoreCollector(d LiveScoreCollectorDeps) *LiveScoreCollector {
	return &LiveScoreCollector{
		fotmob: d.FotMob, espn: d.ESPN, flashscore: d.Flashscore, bo3: d.Bo3, opendota: d.OpenDota, euroleague: d.EuroLeague, acb: d.ACB,
		store: d.Store, redis: d.Redis, sportOf: d.SportOf, log: d.Log, now: d.Now,
	}
}

func (c *LiveScoreCollector) Run(ctx context.Context) error {
	ticker := time.NewTicker(liveScoreInterval)
	defer ticker.Stop()
	for {
		c.CollectRound(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (c *LiveScoreCollector) CollectRound(ctx context.Context) LiveRound {
	now := c.now()
	round := LiveRound{}
	if finished, err := c.store.FinishOverdueEvents(ctx, now, overdueAfterKickoff, overdueFallback, overdueQuietFor); err != nil {
		recordJobFailure(c.log, "postgres", "finish_overdue", "err", err)
	} else if finished > 0 {
		c.log.Info("finished overdue events", "count", finished)
	}
	events, err := c.liveCandidates(ctx, now)
	if err != nil {
		recordJobFailure(c.log, "postgres", "live_scores", "err", err)
		return round
	}
	for feed, feedEvents := range eventsByLiveFeed(events) {
		for _, e := range feedEvents {
			round.countOf(c.sportOf(e.LeagueCode)).Polled++
		}
		feedCtx, cancel := context.WithTimeout(ctx, jobItemBudget)
		scores, err := c.fetchLiveScores(feedCtx, feed, feedEvents, now)
		cancel()
		if err != nil {
			recordJobFailure(c.log, feed, "live_scores", "err", err)
		}
		for _, s := range scores {
			count := round.countOf(c.sportOf(s.event.LeagueCode))
			count.Scored++
			changed, err := c.saveAndPublish(ctx, s, now)
			if err != nil {
				recordJobFailure(c.log, feed, "live_scores", "event_id", s.event.ID, "err", err)
			}
			if changed {
				count.Updated++
			}
		}
	}
	c.log.Info("live scores round", "candidates", len(events))
	for sport, count := range round {
		c.log.Info("live scores by sport", "sport", sport, "polled", count.Polled, "scored", count.Scored, "updated", count.Updated)
	}
	return round
}

func (c *LiveScoreCollector) liveCandidates(ctx context.Context, now time.Time) ([]SourcedEvent, error) {
	from, to := now.Add(-liveScoreLookback), now.Add(liveScoreLookahead)
	linked, err := c.store.EventsAwaitingLiveScores(ctx, from, to, liveScoreSources)
	if err != nil {
		return nil, err
	}
	unlinked, err := c.store.EventsWithoutSourceAwaitingLiveScores(ctx, from, to, canon.SportFootball, fotmob.SourceName)
	if err != nil {
		return nil, err
	}
	for i := range unlinked {
		unlinked[i].Source = unlinkedFootball
	}
	return append(linked, unlinked...), nil
}

func (r LiveRound) countOf(sport canon.Sport) *LiveSportCount {
	if r[sport] == nil {
		r[sport] = &LiveSportCount{}
	}
	return r[sport]
}

func liveFeedOf(e SourcedEvent) (string, bool) {
	switch e.Source {
	case fotmob.SourceName, flashscore.SourceName, acb.SourceName, unlinkedFootball:
		return e.Source, true
	case espn.SourceName:
		_, supported := espn.Leagues[e.LeagueCode]
		return e.Source, supported
	case euroleague.SourceName:
		_, supported := euroleague.Competitions[e.LeagueCode]
		return e.Source, supported
	case bo3.SourceName, esports.LiquipediaSource:
		if e.LeagueCode == dotaLeagueCode {
			return esports.OpenDotaSource, true
		}
		return bo3.SourceName, bo3.Supports(e.LeagueCode)
	}
	return "", false
}

func eventsByLiveFeed(events []SourcedEvent) map[string][]SourcedEvent {
	chosen := map[string]SourcedEvent{}
	var order []string
	for _, e := range events {
		feed, ok := liveFeedOf(e)
		if !ok {
			continue
		}
		previous, seen := chosen[e.ID]
		if !seen {
			order = append(order, e.ID)
		}
		if !seen || (e.Source == feed && previous.Source != feed) {
			chosen[e.ID] = e
		}
	}
	byFeed := map[string][]SourcedEvent{}
	for _, id := range order {
		feed, _ := liveFeedOf(chosen[id])
		byFeed[feed] = append(byFeed[feed], chosen[id])
	}
	return byFeed
}

func (c *LiveScoreCollector) fetchLiveScores(ctx context.Context, feed string, events []SourcedEvent, now time.Time) ([]liveScore, error) {
	switch feed {
	case fotmob.SourceName:
		return c.footballScores(ctx, events)
	case unlinkedFootball:
		return c.unlinkedFootballScores(ctx, events, now)
	case espn.SourceName:
		return c.espnScores(ctx, events)
	case euroleague.SourceName:
		return c.euroLeagueScores(ctx, events)
	case acb.SourceName:
		return c.acbScores(ctx, events)
	case flashscore.SourceName:
		return c.tennisScores(ctx, events)
	case bo3.SourceName:
		return c.esportsScores(ctx, events)
	case esports.OpenDotaSource:
		return c.dotaScores(ctx, events)
	}
	return nil, fmt.Errorf("%s: %w", feed, errUnknownLiveFeed)
}

func (c *LiveScoreCollector) footballScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	byID := map[string]fotmob.LiveMatch{}
	var errs []error
	for _, d := range scheduleDays(events, 0) {
		matches, err := c.fotmob.DayMatches(ctx, d)
		errs = append(errs, err)
		for _, m := range matches {
			byID[m.ID] = m
		}
	}
	var scores []liveScore
	for _, e := range events {
		if m, found := byID[e.SourceID]; found && m.Started {
			scores = append(scores, liveScore{event: e, score: m.Score, finished: m.Finished})
		}
	}
	return scores, errors.Join(errs...)
}

func (c *LiveScoreCollector) espnScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	var scores []liveScore
	var errs []error
	for leagueCode, leagueEvents := range eventsByLeague(events) {
		games := map[string]espn.Game{}
		for _, d := range scheduleDays(leagueEvents, usScheduleDateOffsets...) {
			board, err := c.espn.Games(ctx, leagueCode, d)
			errs = append(errs, err)
			for _, g := range board.Games {
				games[g.ID] = g
			}
		}
		for _, e := range leagueEvents {
			_, gameID, _ := strings.Cut(e.SourceID, ":")
			if g, found := games[gameID]; found && (g.IsLive() || g.IsFinished()) {
				scores = append(scores, liveScore{event: e, score: g.Score, finished: g.IsFinished()})
			}
		}
	}
	return scores, errors.Join(errs...)
}

func (c *LiveScoreCollector) tennisScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	byID := map[string]flashscore.Match{}
	var errs []error
	for _, offset := range flashscoreLiveDayOffsets {
		matches, err := c.flashscore.DayMatches(ctx, offset)
		errs = append(errs, err)
		for _, m := range matches {
			byID[m.ID] = m
		}
	}
	var scores []liveScore
	for _, e := range events {
		m, found := byID[e.SourceID]
		if found && (m.Status == flashscore.StatusLive || flashscore.IsOver(m.Status)) {
			scores = append(scores, liveScore{event: e, score: m.Score, finished: flashscore.IsOver(m.Status)})
		}
	}
	return scores, errors.Join(errs...)
}

func (c *LiveScoreCollector) esportsScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	var scores []liveScore
	var errs []error
	for leagueCode, leagueEvents := range eventsByLeague(events) {
		first, last := kickoffRange(leagueEvents)
		series, err := c.bo3.Matches(ctx, leagueCode, first.Add(-seriesGameLeadWindow), last.Add(seriesGameLeadWindow))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, e := range leagueEvents {
			match, sides, err := seriesWithResolvedSides(ctx, c.store, e, seriesCandidatesFor(e, series))
			switch {
			case errors.Is(err, errSeriesNotFound):
			case err != nil:
				errs = append(errs, err)
			case match.IsLive() || match.IsDecided():
				scores = append(scores, liveScore{event: e, score: seriesScore(match, sides), finished: match.IsDecided()})
			}
		}
	}
	return scores, errors.Join(errs...)
}

func seriesCandidatesFor(e SourcedEvent, series []bo3.Match) []bo3.Match {
	_, slug, hasSlug := strings.Cut(e.SourceID, ":")
	var candidates []bo3.Match
	for _, m := range series {
		if e.Source == bo3.SourceName && hasSlug {
			if m.Slug == slug {
				return []bo3.Match{m}
			}
			continue
		}
		if m.Start.Sub(e.Kickoff).Abs() <= seriesGameLeadWindow {
			candidates = append(candidates, m)
		}
	}
	return candidates
}

func seriesScore(m bo3.Match, sides sideByTeamID) canon.EventScore {
	home, away := m.Maps[0], m.Maps[1]
	if sides[m.Team1.ID] == sideAway {
		home, away = away, home
	}
	score := canon.EventScore{Home: home, Away: away, Period: canon.PeriodFinished}
	if m.IsLive() {
		score.Period = mapPeriodPrefix + strconv.Itoa(home+away+1)
	}
	return score
}

func (c *LiveScoreCollector) saveAndPublish(ctx context.Context, s liveScore, observedAt time.Time) (bool, error) {
	status := seed.EventLive
	if s.finished {
		status = seed.EventFinished
	}
	changed, err := c.store.SaveLiveScore(ctx, s.event.ID, s.score, status, observedAt)
	if err != nil || !changed {
		return false, err
	}
	return true, c.publishScore(ctx, s.event.ID, s.score, observedAt)
}

type boardScoreUpdate struct {
	EventID string           `json:"eventId"`
	Score   canon.EventScore `json:"score"`
}

func (c *LiveScoreCollector) publishScore(ctx context.Context, eventID string, score canon.EventScore, sentAt time.Time) error {
	eventUpdate, err := json.Marshal(canon.BoardUpdate{EventID: eventID, Score: &score, SentAt: sentAt})
	if err != nil {
		return fmt.Errorf("encode score update: %w", err)
	}
	boardUpdate, err := json.Marshal(boardScoreUpdate{EventID: eventID, Score: score})
	if err != nil {
		return fmt.Errorf("encode board score update: %w", err)
	}
	_, err = c.redis.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
		pipe.Publish(ctx, redis.EventChannel(eventID), eventUpdate)
		pipe.Publish(ctx, redis.ChannelBoard, boardUpdate)
		return nil
	})
	if err != nil {
		return fmt.Errorf("publish score update for event %s: %w", eventID, err)
	}
	return nil
}

func eventsByLeague(events []SourcedEvent) map[string][]SourcedEvent {
	byLeague := map[string][]SourcedEvent{}
	for _, e := range events {
		byLeague[e.LeagueCode] = append(byLeague[e.LeagueCode], e)
	}
	return byLeague
}

func kickoffRange(events []SourcedEvent) (time.Time, time.Time) {
	byKickoff := func(a, b SourcedEvent) int { return a.Kickoff.Compare(b.Kickoff) }
	return slices.MinFunc(events, byKickoff).Kickoff, slices.MaxFunc(events, byKickoff).Kickoff
}

func scheduleDays(events []SourcedEvent, offsets ...time.Duration) []time.Time {
	seen := map[time.Time]bool{}
	var days []time.Time
	for _, e := range events {
		for _, offset := range offsets {
			d := e.Kickoff.Add(offset).UTC().Truncate(oneDay)
			if !seen[d] {
				seen[d] = true
				days = append(days, d)
			}
		}
	}
	return days
}
