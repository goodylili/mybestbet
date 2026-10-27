package resolve

import (
	"context"
	"errors"
	"fmt"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

var errUnknownAliasKind = errors.New("resolve: unknown alias kind")

func SubscribeAliasUpdates(ctx context.Context, rdb *goredis.Client) (*goredis.PubSub, error) {
	sub := rdb.Subscribe(ctx, redis.ChannelAliasUpdate)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, fmt.Errorf("subscribe %s: %w", redis.ChannelAliasUpdate, err)
	}
	return sub, nil
}

func (r *Resolver) FollowAliasUpdates(ctx context.Context, sub *goredis.PubSub, afterApply func(context.Context, canon.AliasUpdate)) error {
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, open := <-messages:
			if !open {
				return fmt.Errorf("%s subscription closed", redis.ChannelAliasUpdate)
			}
			r.applyAliasMessage(ctx, msg.Payload, afterApply)
		}
	}
}

func (r *Resolver) applyAliasMessage(ctx context.Context, payload string, afterApply func(context.Context, canon.AliasUpdate)) {
	var u canon.AliasUpdate
	if err := json.Unmarshal([]byte(payload), &u); err != nil {
		r.log.Warn("alias update undecodable", "error", err)
		return
	}
	if err := r.ApplyAliasUpdate(ctx, u); err != nil {
		r.log.Warn("alias update not applied", "source", u.Source, "kind", u.Kind, "source_id", u.SourceID, "error", err)
		return
	}
	if afterApply != nil {
		afterApply(ctx, u)
	}
}

func (r *Resolver) ApplyAliasUpdate(ctx context.Context, u canon.AliasUpdate) error {
	var err error
	switch u.Kind {
	case canon.AliasTeam:
		err = r.ensureTeamCached(ctx, u.TargetID)
		if err == nil {
			r.cache.addTeamAlias(u.Source, u.SourceID, u.TargetID)
		}
	case canon.AliasEvent:
		err = r.ensureEventCached(ctx, u.TargetID)
		if err == nil {
			r.cache.linkEvent(u.Source, u.SourceID, EventLink{EventID: u.TargetID, Flipped: u.Flipped})
		}
	case canon.AliasPlayer:
		err = r.ensurePlayerCached(ctx, u.TargetID)
		if err == nil {
			r.cache.addPlayerAlias(u.Source, u.SourceID, u.TargetID)
		}
	case canon.AliasPlayerMerge:
		err = r.absorbMergedPlayer(ctx, u.SourceID, u.TargetID)
	case aliasEventMerge:
		err = r.ensureEventCached(ctx, u.TargetID)
		if err == nil {
			r.cache.mergeEvent(u.SourceID, EventLink{EventID: u.TargetID, Flipped: u.Flipped})
		}
		return err
	default:
		return fmt.Errorf("%w: %q", errUnknownAliasKind, u.Kind)
	}
	if err != nil {
		return err
	}
	r.enqueued.Delete(reviewKey(u.Kind, u.Source, u.SourceID))
	return nil
}

func (r *Resolver) ensureTeamCached(ctx context.Context, id string) error {
	if _, ok := r.cache.team(id); ok {
		return nil
	}
	t, err := r.store.teamByID(ctx, id)
	if err != nil {
		return err
	}
	r.cache.addTeam(t)
	return r.store.loadLeaguesOfTeam(ctx, r.cache, id)
}

func (r *Resolver) ensureEventCached(ctx context.Context, id string) error {
	if _, ok := r.cache.event(id); ok {
		return nil
	}
	e, err := r.store.eventByID(ctx, id)
	if err != nil {
		return err
	}
	r.cache.addEvent(e)
	return nil
}

func (r *Resolver) ensurePlayerCached(ctx context.Context, id string) error {
	if r.cache.hasPlayer(id) {
		return nil
	}
	p, err := r.store.playerByID(ctx, id)
	if err != nil {
		return err
	}
	r.cache.addPlayer(p)
	return r.store.loadTeamsOfPlayer(ctx, r.cache, id)
}

func (r *Resolver) absorbMergedPlayer(ctx context.Context, duplicateID, survivorID string) error {
	if err := r.ensurePlayerCached(ctx, survivorID); err != nil {
		return err
	}
	r.cache.mergePlayer(duplicateID, survivorID)
	return r.store.loadTeamsOfPlayer(ctx, r.cache, survivorID)
}
