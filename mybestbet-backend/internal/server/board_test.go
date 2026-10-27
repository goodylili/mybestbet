package server

import (
	"context"
	"log/slog"
	"strconv"
	"testing"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func TestBoardCellsLoadsEveryBatchInOrder(t *testing.T) {
	_, rdb := integrationDeps(t)
	ctx := context.Background()
	eventID := uuid.NewString()
	marketKeys := make([]string, 3*coverageReadBatch+7)
	boardKeys := make([]string, len(marketKeys))
	for i := range marketKeys {
		line := float64(i) + 0.5
		marketKeys[i] = canon.MarketKey{EventID: eventID, Code: canon.CodeOU, Line: &line}.String()
		boardKeys[i] = redis.BoardKey(marketKeys[i])
		encoded, err := json.Marshal(canon.BoardCell{MarketKey: marketKeys[i], Source: "src-" + strconv.Itoa(i), Side: canon.SideOver})
		require.NoError(t, err)
		require.NoError(t, rdb.HSet(ctx, boardKeys[i], redis.BoardField("src-"+strconv.Itoa(i), string(canon.SideOver), ""), encoded).Err())
	}
	t.Cleanup(func() { _ = rdb.Del(context.Background(), boardKeys...).Err() })
	s := New(Deps{Config: config.Config{}, Redis: rdb, Log: slog.Default()})

	cells, err := s.boardCells(ctx, marketKeys)
	require.NoError(t, err)
	require.Len(t, cells, len(marketKeys))
	for i, cell := range cells {
		assert.Equal(t, marketKeys[i], cell.MarketKey)
		assert.Equal(t, "src-"+strconv.Itoa(i), cell.Source)
	}

	require.NoError(t, rdb.Del(ctx, boardKeys...).Err())
	cached, err := s.boardCells(ctx, marketKeys)
	require.NoError(t, err)
	assert.Equal(t, cells, cached)
}

func TestBoardCellsReportsRedisFailure(t *testing.T) {
	_, rdb := integrationDeps(t)
	s := New(Deps{Config: config.Config{}, Redis: rdb, Log: slog.Default()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	keys := make([]string, 2*coverageReadBatch)
	for i := range keys {
		keys[i] = uuid.NewString() + "|1X2|||"
	}
	_, err := s.boardCells(ctx, keys)
	assert.ErrorIs(t, err, context.Canceled)
}
