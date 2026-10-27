package booking

import (
	"context"
	"fmt"
	"log/slog"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

type eventLink struct {
	sourceEventID string
	flipped       bool
}

type linkKey struct {
	source  string
	eventID string
}

type slipData struct {
	events  map[string]bool
	links   map[linkKey]eventLink
	players map[string]string
	cells   map[string]map[string]canon.BoardCell
}

func (d slipData) cell(leg slipLeg, source string) (canon.BoardCell, bool) {
	cell, ok := d.cells[leg.keyStr][fieldOf(source, leg)]
	return cell, ok
}

func fieldOf(source string, leg slipLeg) string {
	return redis.BoardField(source, string(leg.side), leg.label)
}

type store struct {
	pool *pgxpool.Pool
	rdb  *goredis.Client
	log  *slog.Logger
}

func (s store) load(ctx context.Context, legs []slipLeg, withLinks bool) (slipData, error) {
	eventIDs, marketKeys, playerIDs := distinctRefs(legs)
	data := slipData{links: map[linkKey]eventLink{}, players: map[string]string{}}
	var err error
	if data.events, err = s.knownEvents(ctx, eventIDs); err != nil {
		return slipData{}, err
	}
	if data.cells, err = s.boardCells(ctx, marketKeys); err != nil {
		return slipData{}, err
	}
	if !withLinks {
		return data, nil
	}
	if data.links, err = s.eventLinks(ctx, eventIDs); err != nil {
		return slipData{}, err
	}
	if data.players, err = s.playerNames(ctx, playerIDs); err != nil {
		return slipData{}, err
	}
	return data, nil
}

func distinctRefs(legs []slipLeg) (eventIDs, marketKeys, playerIDs []string) {
	seen := map[string]bool{}
	add := func(into *[]string, value string) {
		if value != "" && !seen[value] {
			seen[value] = true
			*into = append(*into, value)
		}
	}
	for _, leg := range legs {
		add(&eventIDs, leg.key.EventID)
		add(&marketKeys, leg.keyStr)
		add(&playerIDs, leg.key.PlayerID)
	}
	return eventIDs, marketKeys, playerIDs
}

func (s store) knownEvents(ctx context.Context, eventIDs []string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM events WHERE id = ANY($1::uuid[])`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()
	known := make(map[string]bool, len(eventIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		known[id] = true
	}
	return known, rows.Err()
}

func (s store) eventLinks(ctx context.Context, eventIDs []string) (map[linkKey]eventLink, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (source, "eventId") source, "eventId"::text, "sourceEventId", flipped
		FROM event_links WHERE "eventId" = ANY($1::uuid[]) ORDER BY source, "eventId", "createdAt" DESC`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("query event links: %w", err)
	}
	defer rows.Close()
	links := map[linkKey]eventLink{}
	for rows.Next() {
		var key linkKey
		var link eventLink
		if err := rows.Scan(&key.source, &key.eventID, &link.sourceEventID, &link.flipped); err != nil {
			return nil, fmt.Errorf("scan event link: %w", err)
		}
		links[key] = link
	}
	return links, rows.Err()
}

func (s store) playerNames(ctx context.Context, playerIDs []string) (map[string]string, error) {
	names := make(map[string]string, len(playerIDs))
	if len(playerIDs) == 0 {
		return names, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, name FROM players WHERE id = ANY($1::uuid[])`, playerIDs)
	if err != nil {
		return nil, fmt.Errorf("query players: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		names[id] = name
	}
	return names, rows.Err()
}

func (s store) boardCells(ctx context.Context, marketKeys []string) (map[string]map[string]canon.BoardCell, error) {
	pipe := s.rdb.Pipeline()
	hashes := make([]*goredis.MapStringStringCmd, len(marketKeys))
	for i, key := range marketKeys {
		hashes[i] = pipe.HGetAll(ctx, redis.BoardKey(key))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("read board: %w", err)
	}
	cells := make(map[string]map[string]canon.BoardCell, len(marketKeys))
	for i, key := range marketKeys {
		decoded := make(map[string]canon.BoardCell, len(hashes[i].Val()))
		for field, encoded := range hashes[i].Val() {
			var cell canon.BoardCell
			if err := json.Unmarshal([]byte(encoded), &cell); err != nil {
				s.log.WarnContext(ctx, "skip undecodable board cell", "market_key", key, "field", field, "err", err)
				continue
			}
			decoded[field] = cell
		}
		cells[key] = decoded
	}
	return cells, nil
}
