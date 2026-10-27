package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mybestbet/internal/loadtest"
	"mybestbet/internal/platform/config"
)

const hotTierName = "hot"

var errNoHotTier = errors.New("config has no hot tier")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	hotInterval, err := hotTierInterval(cfg)
	if err != nil {
		return err
	}
	ltCfg, err := parseFlags(args, hotInterval)
	if err != nil {
		return err
	}
	result, err := loadtest.Run(ctx, ltCfg)
	if err != nil {
		return err
	}
	if err := writeResult(out, ltCfg, result); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return loadtest.Verdict(result.Reports)
}

func parseFlags(args []string, hotInterval time.Duration) (loadtest.Config, error) {
	cfg := loadtest.Config{HotInterval: hotInterval}
	flags := flag.NewFlagSet("loadtest", flag.ContinueOnError)
	flags.StringVar(&cfg.RedisURL, "redis", "redis://localhost:6379/14", "redis url of the engine under test")
	flags.StringVar(&cfg.APIURL, "api-url", "", "api base url; when set, the SSE stream is measured too")
	flags.StringVar(&cfg.TestdataDir, "testdata", "testdata", "directory holding <source>/golden/*price.json")
	flags.IntVar(&cfg.Events, "events", 40, "synthetic hot-tier events")
	flags.IntVar(&cfg.SSEEvents, "sse-events", 4, "events watched over SSE when -api-url is set")
	flags.DurationVar(&cfg.Duration, "duration", time.Minute, "how long to replay")
	flags.Float64Var(&cfg.RateMultiplier, "rate", 10, "multiple of the hot-tier poll rate")
	flags.Float64Var(&cfg.MaxJitter, "jitter", 0.01, "max relative price move per resend, independent per selection")
	flags.DurationVar(&cfg.DropAfter, "drop-after", 5*time.Second, "a delta not seen within this is dropped")
	if err := flags.Parse(args); err != nil {
		return loadtest.Config{}, err
	}
	if cfg.Events <= 0 || cfg.RateMultiplier <= 0 || cfg.Duration <= 0 || cfg.DropAfter <= 0 || cfg.MaxJitter < 0 || cfg.MaxJitter >= 1 {
		return loadtest.Config{}, errors.New("events, rate, duration and drop-after must be positive and jitter in [0, 1)")
	}
	return cfg, nil
}

func hotTierInterval(cfg config.Config) (time.Duration, error) {
	for _, tier := range cfg.Tiers {
		if tier.Name == hotTierName && tier.Interval > 0 {
			return tier.Interval, nil
		}
	}
	return 0, errNoHotTier
}

func writeResult(out io.Writer, cfg loadtest.Config, result loadtest.Result) error {
	var b strings.Builder
	fmt.Fprintf(&b, "replayed %d sources x %d events = %d pairs, each every %s (%gx hot tier %s), %.0f deltas/s for %s, jitter ±%g%%\n",
		result.Sources, cfg.Events, result.Pairs, result.Period, cfg.RateMultiplier, cfg.HotInterval, result.DeltasPerS, cfg.Duration, cfg.MaxJitter*100)
	fmt.Fprintf(&b, "latency = XADD odds.delta -> BoardUpdate cell received; threshold p95 <= %s, zero drops\n", loadtest.MaxInternalP95)
	for _, r := range result.Reports {
		fmt.Fprintln(&b, r)
	}
	if result.SSEMeasured {
		b.WriteString("sse = measured end to end through the api's /v1/stream, including its 250ms coalescing window\n")
	} else {
		b.WriteString("measured at redis pub/sub only: the api's 250ms SSE coalescing window is not included (pass -api-url to measure it)\n")
	}
	_, err := io.WriteString(out, b.String())
	return err
}
