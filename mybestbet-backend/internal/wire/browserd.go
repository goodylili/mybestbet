package wire

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mybestbet/internal/browserd"
	"mybestbet/internal/platform/observability"
)

const (
	defaultBrowserdAddr         = ":9222"
	defaultBrowserdPoolSize     = 2
	defaultBrowserdAllowedHosts = "bet365.com,prizepicks.com,sofascore.com,fbref.com"
	browserdMaxWait             = 60 * time.Second
)

func RunBrowserd(ctx context.Context, log *slog.Logger) error {
	poolSize, err := browserdPoolSizeFromEnv()
	if err != nil {
		return err
	}
	pool := browserd.NewPool(browserd.Options{
		ProfileRoot: browserdEnvOr("BROWSERD_PROFILE_DIR", filepath.Join(os.TempDir(), "browserd-profiles")),
		Size:        poolSize,
		ChromeBin:   os.Getenv("BROWSERD_CHROME_BIN"),
	}, log)
	defer pool.Close()

	policy := browserd.RequestPolicy{
		AllowedHostSuffixes: strings.Split(browserdEnvOr("BROWSERD_ALLOWED_HOSTS", defaultBrowserdAllowedHosts), ","),
		MaxWait:             browserdMaxWait,
	}
	mux := observability.Mux()
	mux.Handle("POST /fetch", browserd.NewFetchHandler(pool, policy, log))
	return observability.Serve(ctx, browserdEnvOr("BROWSERD_ADDR", defaultBrowserdAddr), mux, log)
}

func browserdPoolSizeFromEnv() (int, error) {
	raw := os.Getenv("BROWSERD_POOL")
	if raw == "" {
		return defaultBrowserdPoolSize, nil
	}
	size, err := strconv.Atoi(raw)
	if err != nil || size < 1 {
		return 0, fmt.Errorf("BROWSERD_POOL %q: want a positive integer", raw)
	}
	return size, nil
}

func browserdEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
