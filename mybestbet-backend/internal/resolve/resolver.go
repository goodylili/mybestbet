package resolve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/review"
	"mybestbet/pkg/canon"
)

const eventCacheLookback = 48 * time.Hour

var ErrNotDelivered = errors.New("resolve: markets not delivered")

type Resolver struct {
	cache   *Cache
	store   *store
	pending *pendingBuffer
	reviews *review.Service
	rdb     *goredis.Client
	log     *slog.Logger
	now     func() time.Time

	eventCreation sync.Mutex
	enqueued      sync.Map
}

func New(pool *pgxpool.Pool, rdb *goredis.Client, reviews *review.Service, log *slog.Logger) (*Resolver, error) {
	cache, err := NewCache()
	if err != nil {
		return nil, err
	}
	return &Resolver{
		cache:   cache,
		store:   &store{pool: pool},
		pending: &pendingBuffer{rdb: rdb},
		reviews: reviews,
		rdb:     rdb,
		log:     log,
		now:     func() time.Time { return time.Now().UTC() },
	}, nil
}

func (r *Resolver) WithMatching(m MatchingConfig) *Resolver {
	r.cache.setMatching(m)
	return r
}

func (r *Resolver) Load(ctx context.Context) error {
	start := time.Now()
	if err := r.store.loadInto(ctx, r.cache, r.now().Add(-eventCacheLookback)); err != nil {
		return fmt.Errorf("load alias cache: %w", err)
	}
	r.log.Info("alias cache loaded", "took_ms", time.Since(start).Milliseconds())
	return nil
}

func (r *Resolver) ResolveAndPublish(ctx context.Context, source string, fetchedAt time.Time, markets []canon.RawMarket, polled []PolledEvent) (int, error) {
	start := time.Now()
	deltas, pending, resolveErr := r.resolveMarkets(ctx, source, fetchedAt, markets)
	observeStage(stageResolve, start)
	if err := r.pending.store(ctx, pending); err != nil {
		return 0, errors.Join(resolveErr, fmt.Errorf("%w: %w", ErrNotDelivered, err))
	}
	if err := publishDeltas(ctx, r.rdb, deltas, r.freshness(source, fetchedAt, polled)); err != nil {
		return 0, errors.Join(resolveErr, fmt.Errorf("%w: %w", ErrNotDelivered, err))
	}
	countPending(pending)
	return len(deltas), resolveErr
}

func (r *Resolver) freshness(source string, observedAt time.Time, polled []PolledEvent) []canon.Freshness {
	byEvent := map[string]int{}
	var confirmed []canon.Freshness
	for _, p := range polled {
		link, ok := r.cache.eventLink(source, p.SourceEventID)
		if !ok {
			continue
		}
		if i, seen := byEvent[link.EventID]; seen {
			confirmed[i].SourceKeys = append(confirmed[i].SourceKeys, p.SourceKeys...)
			confirmed[i].Complete = confirmed[i].Complete && p.Complete
			continue
		}
		byEvent[link.EventID] = len(confirmed)
		confirmed = append(confirmed, canon.Freshness{Source: source, EventID: link.EventID, ObservedAt: observedAt, SourceKeys: slices.Clone(p.SourceKeys), Complete: p.Complete})
	}
	return confirmed
}

func (r *Resolver) resolveMarkets(ctx context.Context, source string, fetchedAt time.Time, markets []canon.RawMarket) ([]canon.Delta, []PendingMarket, error) {
	deltas := make([]canon.Delta, 0, len(markets))
	var (
		pending []PendingMarket
		errs    []error
	)
	resolvedAt := r.now()
	for _, m := range markets {
		market, unresolved, err := r.resolveMarket(ctx, source, m)
		if err != nil {
			errs = append(errs, err)
		}
		if unresolved != nil {
			unresolved.FetchedAt = fetchedAt
			pending = append(pending, *unresolved)
			continue
		}
		deltas = append(deltas, canon.Delta{Source: source, Market: market, FetchedAt: fetchedAt, ResolvedAt: resolvedAt})
	}
	return deltas, pending, errors.Join(errs...)
}

func (r *Resolver) resolveMarket(ctx context.Context, source string, m canon.RawMarket) (canon.Market, *PendingMarket, error) {
	link, ok := r.cache.eventLink(source, m.EventSourceID)
	if !ok {
		return canon.Market{}, &PendingMarket{Kind: canon.AliasEvent, Source: source, SourceKey: m.EventSourceID, Market: m}, nil
	}
	playerID := ""
	if m.Player != nil {
		id, resolved, err := r.resolvePlayer(ctx, source, *m.Player, link.EventID)
		if !resolved {
			return canon.Market{}, &PendingMarket{Kind: canon.AliasPlayer, Source: source, SourceKey: playerSourceKey(*m.Player), Market: m}, err
		}
		playerID = id
	}
	teamID := ""
	if m.Team != "" {
		id, resolved := r.eventTeamNamed(source, m.Team, link.EventID)
		if !resolved {
			return canon.Market{}, &PendingMarket{Kind: canon.AliasTeam, Source: source, SourceKey: m.Team, Market: m}, nil
		}
		teamID = id
	}
	return canonicalMarket(m, link, playerID, teamID), nil, nil
}

func (r *Resolver) eventTeamNamed(source, rawTeam, eventID string) (string, bool) {
	ev, ok := r.cache.event(eventID)
	if !ok {
		return "", false
	}
	if id, ok := r.cache.teamAlias(source, rawTeam); ok && ev.involves(id) {
		return id, true
	}
	for _, candidate := range r.cache.rankLeagueTeams(ev.League, rawTeam) {
		if ev.involves(candidate.id) {
			return candidate.id, true
		}
	}
	return "", false
}

func (r *Resolver) resolvePlayer(ctx context.Context, source string, p canon.SourcePlayer, eventID string) (string, bool, error) {
	ev, ok := r.cache.event(eventID)
	if !ok {
		return "", false, nil
	}
	m := r.cache.matchPlayer(source, p, ev)
	switch m.outcome {
	case matchExactAlias:
		return m.id, true, nil
	case matchAutoLinked:
		stored, err := r.store.linkPlayerAlias(ctx, source, playerSourceKey(p), m.id, m.score)
		if err != nil {
			return "", false, err
		}
		r.cache.addPlayerAlias(source, playerSourceKey(p), stored)
		countAutoLink(canon.AliasPlayer)
		return stored, true, r.announce(ctx, canon.AliasUpdate{Kind: canon.AliasPlayer, Source: source, SourceID: playerSourceKey(p), TargetID: stored})
	case matchNeedsReview, matchFixtureOpponent:
	}
	return "", false, r.enqueueReviewOnce(ctx, review.NewItem{
		Kind:       canon.AliasPlayer,
		Source:     source,
		SourceKey:  playerSourceKey(p),
		Payload:    review.PlayerPayload{Player: p, EventID: eventID, TeamID: r.guessPlayerTeam(source, p, ev)},
		Candidates: m.candidates,
	})
}

func (r *Resolver) guessPlayerTeam(source string, p canon.SourcePlayer, ev Event) string {
	teamID, ok := r.cache.teamAlias(source, p.Team)
	if ok && ev.involves(teamID) {
		return teamID
	}
	return ""
}

func (r *Resolver) LinkEvents(ctx context.Context, source string, events []canon.SourceEvent) ([]string, error) {
	var (
		linked []string
		errs   []error
	)
	for _, ev := range events {
		ok, err := r.linkEvent(ctx, source, ev)
		if err != nil {
			errs = append(errs, fmt.Errorf("link %s event %s: %w", source, ev.SourceID, err))
		}
		if ok {
			linked = append(linked, ev.SourceID)
		}
	}
	return linked, errors.Join(errs...)
}

func (r *Resolver) LinkedEventID(source, sourceEventID string) (string, bool) {
	link, ok := r.cache.eventLink(source, sourceEventID)
	return link.EventID, ok
}

func (r *Resolver) linkEvent(ctx context.Context, source string, ev canon.SourceEvent) (bool, error) {
	if current, ok := r.cache.eventLink(source, ev.SourceID); ok {
		return true, r.relinkIfKickoffMoved(ctx, source, ev, current)
	}
	if !r.canResolveTeamsIn(ev.LeagueHint) {
		return false, nil
	}
	home, away := r.matchFixtureTeams(source, ev)
	homeID, homeOK, homeErr := r.settleTeam(ctx, source, ev.LeagueHint, ev.Home, ev.SourceID, home)
	awayID, awayOK, awayErr := r.settleTeam(ctx, source, ev.LeagueHint, ev.Away, ev.SourceID, away)
	if !homeOK || !awayOK {
		return false, errors.Join(homeErr, awayErr, r.pending.holdEvent(ctx, source, ev))
	}
	link, found, err := r.findOrCreateEvent(ctx, source, ev, homeID, awayID)
	if err != nil || !found {
		return false, errors.Join(err, r.pending.holdEvent(ctx, source, ev))
	}
	stored, err := r.store.linkEvent(ctx, source, ev.SourceID, link)
	if err != nil {
		return false, err
	}
	r.cache.linkEvent(source, ev.SourceID, stored)
	countAutoLink(canon.AliasEvent)
	return true, errors.Join(
		r.announce(ctx, canon.AliasUpdate{Kind: canon.AliasEvent, Source: source, SourceID: ev.SourceID, TargetID: stored.EventID, Flipped: stored.Flipped}),
		r.pending.releaseEvent(ctx, source, ev.SourceID),
	)
}

func (r *Resolver) relinkIfKickoffMoved(ctx context.Context, source string, ev canon.SourceEvent, current EventLink) error {
	linked, ok := r.cache.event(current.EventID)
	if !ok || absDuration(linked.Kickoff.Sub(ev.Kickoff)) <= r.cache.kickoffToleranceIn(linked.League) {
		return nil
	}
	homeID, awayID := linked.HomeTeamID, linked.AwayTeamID
	if current.Flipped {
		homeID, awayID = awayID, homeID
	}
	candidates := r.cache.eventCandidates(linked.League, homeID, awayID, ev.Kickoff)
	if len(candidates) != 1 {
		return nil
	}
	target := EventLink{EventID: candidates[0].event.ID, Flipped: candidates[0].flipped}
	if err := r.store.relinkEvent(ctx, source, ev.SourceID, target); err != nil {
		return err
	}
	r.cache.linkEvent(source, ev.SourceID, target)
	countRelink()
	r.log.Info("event link moved to matching kickoff", "source", source, "source_event_id", ev.SourceID, "from", current.EventID, "to", target.EventID)
	return r.announce(ctx, canon.AliasUpdate{Kind: canon.AliasEvent, Source: source, SourceID: ev.SourceID, TargetID: target.EventID, Flipped: target.Flipped})
}

func (r *Resolver) canResolveTeamsIn(league string) bool {
	if r.cache.sportOf(league) == canon.SportTennis {
		return r.cache.isKnownLeague(league)
	}
	return r.cache.hasLeagueTeams(league)
}

func (r *Resolver) matchFixtureTeams(source string, ev canon.SourceEvent) (match, match) {
	league := ev.LeagueHint
	home := r.cache.matchTeam(source, league, ev.Home)
	away := r.cache.matchTeam(source, league, ev.Away)
	if r.cache.sportOf(league) == canon.SportTennis || home.isSettled() == away.isSettled() {
		return home, away
	}
	if home.isSettled() {
		return home, r.cache.inferFixtureOpponent(league, ev.Kickoff, home.id, ev.Away, away)
	}
	return r.cache.inferFixtureOpponent(league, ev.Kickoff, away.id, ev.Home, home), away
}

func (r *Resolver) settleTeam(ctx context.Context, source, league, name, sourceEventID string, m match) (string, bool, error) {
	switch m.outcome {
	case matchExactAlias:
		return m.id, true, r.joinTennisLeague(ctx, m.id, league)
	case matchAutoLinked:
		return r.autoLinkTeam(ctx, source, league, name, m)
	case matchFixtureOpponent:
		countFixtureInference()
		return m.id, true, nil
	case matchNeedsReview:
	}
	if r.cache.sportOf(league) == canon.SportTennis && isNewCompetitorCandidate(m, name) {
		return r.createCompetitor(ctx, source, league, name)
	}
	return "", false, r.enqueueReviewOnce(ctx, review.NewItem{
		Kind:       canon.AliasTeam,
		Source:     source,
		SourceKey:  name,
		Payload:    review.TeamPayload{League: league, Name: name, SourceEventID: sourceEventID},
		Candidates: m.candidates,
	})
}

func (r *Resolver) autoLinkTeam(ctx context.Context, source, league, name string, m match) (string, bool, error) {
	stored, err := r.store.linkTeamAlias(ctx, source, name, m.id, m.score)
	if err != nil {
		return "", false, err
	}
	if stored != m.id {
		return m.id, true, r.joinTennisLeague(ctx, m.id, league)
	}
	r.cache.addTeamAlias(source, name, stored)
	countAutoLink(canon.AliasTeam)
	return stored, true, errors.Join(
		r.joinTennisLeague(ctx, stored, league),
		r.announce(ctx, canon.AliasUpdate{Kind: canon.AliasTeam, Source: source, SourceID: name, TargetID: stored}),
	)
}

func (r *Resolver) joinTennisLeague(ctx context.Context, teamID, league string) error {
	if r.cache.sportOf(league) != canon.SportTennis || r.cache.isLeagueMember(teamID, league) {
		return nil
	}
	if err := r.store.joinLeague(ctx, teamID, league); err != nil {
		return err
	}
	r.cache.joinLeague(teamID, league)
	return nil
}

func (r *Resolver) createCompetitor(ctx context.Context, source, league, name string) (string, bool, error) {
	r.eventCreation.Lock()
	defer r.eventCreation.Unlock()
	if m := r.cache.matchTeam(source, league, name); m.outcome != matchNeedsReview {
		return m.id, true, nil
	}
	team, player, err := r.store.insertCompetitor(ctx, source, league, name)
	if err != nil {
		return "", false, err
	}
	r.cache.addTeam(team)
	r.cache.addPlayer(player)
	r.cache.addTeamAlias(source, name, team.ID)
	countAutoLink(canon.AliasTeam)
	return team.ID, true, r.announce(ctx, canon.AliasUpdate{Kind: canon.AliasTeam, Source: source, SourceID: name, TargetID: team.ID})
}

func (r *Resolver) findOrCreateEvent(ctx context.Context, source string, ev canon.SourceEvent, homeID, awayID string) (EventLink, bool, error) {
	candidates := r.cache.fixtureCandidates(ev.LeagueHint, homeID, awayID, ev.Kickoff)
	switch len(candidates) {
	case 0:
		return r.createEvent(ctx, Event{League: ev.LeagueHint, HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: ev.Kickoff})
	case 1:
		return EventLink{EventID: candidates[0].event.ID, Flipped: candidates[0].flipped}, true, nil
	}
	return EventLink{}, false, r.enqueueReviewOnce(ctx, review.NewItem{
		Kind:       canon.AliasEvent,
		Source:     source,
		SourceKey:  ev.SourceID,
		Payload:    review.EventPayload{Event: ev, League: ev.LeagueHint, HomeTeamID: homeID, AwayTeamID: awayID},
		Candidates: topCandidates(r.cache.eventCandidateScores(ev.LeagueHint, candidates, ev.Kickoff)),
	})
}

func (r *Resolver) createEvent(ctx context.Context, e Event) (EventLink, bool, error) {
	r.eventCreation.Lock()
	defer r.eventCreation.Unlock()
	if existing := r.cache.fixtureCandidates(e.League, e.HomeTeamID, e.AwayTeamID, e.Kickoff); len(existing) > 0 {
		return EventLink{EventID: existing[0].event.ID, Flipped: existing[0].flipped}, true, nil
	}
	id, err := r.store.upsertEvent(ctx, e)
	if err != nil {
		return EventLink{}, false, err
	}
	e.ID = id
	r.cache.addEvent(e)
	return EventLink{EventID: id}, true, nil
}

func (r *Resolver) announce(ctx context.Context, u canon.AliasUpdate) error {
	return publishAliasUpdate(ctx, r.rdb, u)
}

func reviewKey(kind, source, sourceKey string) string { return kind + "|" + source + "|" + sourceKey }

func (r *Resolver) enqueueReviewOnce(ctx context.Context, it review.NewItem) error {
	key := reviewKey(it.Kind, it.Source, it.SourceKey)
	if _, already := r.enqueued.Load(key); already {
		return nil
	}
	if err := r.reviews.Enqueue(ctx, it); err != nil {
		return err
	}
	r.enqueued.Store(key, struct{}{})
	countReview(it.Kind)
	return nil
}
