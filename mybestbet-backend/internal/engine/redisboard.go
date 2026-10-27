package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const marketKeySeparator = "|"

type cellWrite struct {
	cell      canon.BoardCell
	isRemoved bool
}

type eventWrites struct {
	published      []canon.BoardCell
	latestByMarket map[string]map[cellKey]cellWrite
}

type boardWrites map[string]*eventWrites

func mergeBoardWrites(changes []changeSet) boardWrites {
	writes := boardWrites{}
	for _, cs := range changes {
		for _, c := range cs.cells {
			writes.of(c).record(c, false)
		}
		for _, c := range cs.removed {
			writes.of(c).record(c, true)
		}
	}
	return writes
}

func (w boardWrites) of(c canon.BoardCell) *eventWrites {
	eventID, _, _ := strings.Cut(c.MarketKey, marketKeySeparator)
	ev := w[eventID]
	if ev == nil {
		ev = &eventWrites{latestByMarket: map[string]map[cellKey]cellWrite{}}
		w[eventID] = ev
	}
	return ev
}

func (ev *eventWrites) record(c canon.BoardCell, isRemoved bool) {
	ev.published = append(ev.published, c)
	latest := ev.latestByMarket[c.MarketKey]
	if latest == nil {
		latest = map[cellKey]cellWrite{}
		ev.latestByMarket[c.MarketKey] = latest
	}
	latest[cellKeyOf(c)] = cellWrite{cell: c, isRemoved: isRemoved}
}

func (w boardWrites) queue(ctx context.Context, pipe goredis.Pipeliner, now time.Time) error {
	for eventID, ev := range w {
		if err := ev.queue(ctx, pipe, eventID, now); err != nil {
			return err
		}
	}
	return nil
}

func (ev *eventWrites) queue(ctx context.Context, pipe goredis.Pipeliner, eventID string, now time.Time) error {
	var written []any
	for marketKey, latest := range ev.latestByMarket {
		var set []any
		var removed []string
		for key, w := range latest {
			if w.isRemoved {
				removed = append(removed, key.boardField())
				continue
			}
			encoded, err := json.Marshal(w.cell)
			if err != nil {
				return fmt.Errorf("encode board cell %s: %w", marketKey, err)
			}
			set = append(set, key.boardField(), encoded)
		}
		if len(set) > 0 {
			pipe.HSet(ctx, redis.BoardKey(marketKey), set...)
			written = append(written, marketKey)
		}
		if len(removed) > 0 {
			pipe.HDel(ctx, redis.BoardKey(marketKey), removed...)
		}
	}
	if len(written) > 0 {
		pipe.SAdd(ctx, redis.EventMarketsKey(eventID), written...)
	}
	update, err := json.Marshal(canon.BoardUpdate{EventID: eventID, Cells: ev.published, SentAt: now})
	if err != nil {
		return fmt.Errorf("encode board update %s: %w", eventID, err)
	}
	pipe.Publish(ctx, redis.EventChannel(eventID), update)
	return nil
}
