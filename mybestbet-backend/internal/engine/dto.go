package engine

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

type Deps struct {
	Config   config.Config
	Pool     *pgxpool.Pool
	Redis    *goredis.Client
	Log      *slog.Logger
	Group    string
	Consumer string
	SpoolDir string
}

type priceRow struct {
	marketKey       string
	eventID         string
	code            canon.MarketCode
	line            *float64
	playerID        string
	period          string
	cell            canon.BoardCell
	rules           canon.SettleRule
	sourceMarketKey string
	isRawChanged    bool
}

func newPriceRow(ms *marketState, c canon.BoardCell, sq *sourceQuotes) priceRow {
	return priceRow{
		marketKey:       ms.keyString,
		eventID:         ms.key.EventID,
		code:            ms.key.Code,
		line:            ms.key.Line,
		playerID:        ms.key.PlayerID,
		period:          ms.key.Period,
		cell:            c,
		rules:           sq.rules,
		sourceMarketKey: sq.sourceMarketKey,
	}
}
