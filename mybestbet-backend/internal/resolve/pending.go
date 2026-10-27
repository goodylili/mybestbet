package resolve

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/pkg/canon"
)

const (
	pendingMarketTTL     = 6 * time.Hour
	heldEventTTL         = 7 * 24 * time.Hour
	pendingIndexKey      = "resolve:pending:index"
	heldEventSourcesKey  = "resolve:held:sources"
	pendingIndexSplitter = "|"
)

func pendingMarketsKey(kind, source, sourceKey string) string {
	return "resolve:pending:" + kind + ":" + source + ":" + sourceKey
}

func heldEventsKey(source string) string { return "resolve:held:" + source }

type pendingBucket struct {
	kind      string
	source    string
	sourceKey string
}

func (b pendingBucket) indexMember() string {
	return strings.Join([]string{b.kind, b.source, b.sourceKey}, pendingIndexSplitter)
}

func parsePendingBucket(member string) (pendingBucket, bool) {
	parts := strings.SplitN(member, pendingIndexSplitter, 3)
	if len(parts) != 3 {
		return pendingBucket{}, false
	}
	return pendingBucket{kind: parts[0], source: parts[1], sourceKey: parts[2]}, true
}

func (b pendingBucket) key() string { return pendingMarketsKey(b.kind, b.source, b.sourceKey) }

type pendingBuffer struct {
	rdb *goredis.Client
}

func pendingMarketField(m canon.RawMarket) string {
	line := ""
	if m.Line != nil {
		line = strconv.FormatFloat(*m.Line, 'f', -1, 64)
	}
	player := ""
	if m.Player != nil {
		player = playerSourceKey(*m.Player)
	}
	return strings.Join([]string{m.EventSourceID, string(m.Code), line, player, m.Period, m.SourceMarketID}, pendingIndexSplitter)
}

func (p *pendingBuffer) store(ctx context.Context, markets []PendingMarket) error {
	if len(markets) == 0 {
		return nil
	}
	pipe := p.rdb.Pipeline()
	for _, m := range markets {
		body, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("encode pending market: %w", err)
		}
		bucket := pendingBucket{kind: m.Kind, source: m.Source, sourceKey: m.SourceKey}
		pipe.HSet(ctx, bucket.key(), pendingMarketField(m.Market), body)
		pipe.Expire(ctx, bucket.key(), pendingMarketTTL)
		pipe.SAdd(ctx, pendingIndexKey, bucket.indexMember())
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("buffer pending markets: %w", err)
	}
	return nil
}

func (p *pendingBuffer) take(ctx context.Context, bucket pendingBucket) ([]PendingMarket, error) {
	var stored *goredis.MapStringStringCmd
	_, err := p.rdb.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		stored = pipe.HGetAll(ctx, bucket.key())
		pipe.Del(ctx, bucket.key())
		pipe.SRem(ctx, pendingIndexKey, bucket.indexMember())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("take pending markets: %w", err)
	}
	markets := make([]PendingMarket, 0, len(stored.Val()))
	for _, body := range stored.Val() {
		var m PendingMarket
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			return nil, fmt.Errorf("decode pending market: %w", err)
		}
		markets = append(markets, m)
	}
	return markets, nil
}

func (p *pendingBuffer) buckets(ctx context.Context) ([]pendingBucket, error) {
	members, err := p.rdb.SMembers(ctx, pendingIndexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("list pending buckets: %w", err)
	}
	buckets := make([]pendingBucket, 0, len(members))
	for _, member := range members {
		if b, ok := parsePendingBucket(member); ok {
			buckets = append(buckets, b)
		}
	}
	return buckets, nil
}

func (p *pendingBuffer) holdEvent(ctx context.Context, source string, ev canon.SourceEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode held event: %w", err)
	}
	pipe := p.rdb.Pipeline()
	pipe.HSet(ctx, heldEventsKey(source), ev.SourceID, body)
	pipe.Expire(ctx, heldEventsKey(source), heldEventTTL)
	pipe.SAdd(ctx, heldEventSourcesKey, source)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("hold unresolved event: %w", err)
	}
	return nil
}

func (p *pendingBuffer) releaseEvent(ctx context.Context, source, sourceEventID string) error {
	if err := p.rdb.HDel(ctx, heldEventsKey(source), sourceEventID).Err(); err != nil {
		return fmt.Errorf("release held event: %w", err)
	}
	return nil
}

func (p *pendingBuffer) heldEvents(ctx context.Context, source string) ([]canon.SourceEvent, error) {
	stored, err := p.rdb.HGetAll(ctx, heldEventsKey(source)).Result()
	if err != nil {
		return nil, fmt.Errorf("read held events: %w", err)
	}
	events := make([]canon.SourceEvent, 0, len(stored))
	for _, body := range stored {
		var ev canon.SourceEvent
		if err := json.Unmarshal([]byte(body), &ev); err != nil {
			return nil, fmt.Errorf("decode held event: %w", err)
		}
		events = append(events, ev)
	}
	return events, nil
}

func (p *pendingBuffer) sourcesWithHeldEvents(ctx context.Context) ([]string, error) {
	sources, err := p.rdb.SMembers(ctx, heldEventSourcesKey).Result()
	if err != nil {
		return nil, fmt.Errorf("list sources with held events: %w", err)
	}
	return sources, nil
}
