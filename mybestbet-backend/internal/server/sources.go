package server

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/logos"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/sources"
)

const (
	fakeSourceName = "fake"
	feedStream     = "stream"
	feedPoll       = "poll"

	sourceSnapshotRefresh = 3 * time.Second
)

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	health, ok := s.sources.get()
	if !ok {
		var err error
		if health, err = s.sourceHealthList(r.Context()); err != nil {
			s.writeFailure(w, r, err)
			return
		}
	}
	writeCachedJSON(w, r, listOf(health), sourcesMaxAge)
}

func (s *Server) refreshSources(ctx context.Context) error {
	health, err := s.sourceHealthList(ctx)
	if err != nil {
		return err
	}
	s.sources.set(health)
	return nil
}

func (s *Server) sourceHealthList(ctx context.Context) ([]SourceHealth, error) {
	names := s.sourceNames()
	pipe := s.rdb.Pipeline()
	hashes := make([]*goredis.MapStringStringCmd, len(names))
	for i, name := range names {
		hashes[i] = pipe.HGetAll(ctx, redis.SourceHealthKey(name))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	logoURLs, err := s.logoURLs(ctx, logos.KindSource, names)
	if err != nil {
		return nil, err
	}
	coverage, err := s.sourceCoverage(ctx)
	if err != nil {
		return nil, err
	}
	health := make([]SourceHealth, len(names))
	for i, name := range names {
		health[i] = sourceHealthFromHash(name, s.cfg.Sources[name].Enabled, hashes[i].Val())
		health[i].LogoURL = logoURLs[name]
		s.describeSource(&health[i], coverage[name])
	}
	return health, nil
}

func (s *Server) describeSource(health *SourceHealth, coverage SourceCoverage) {
	profile := s.cfg.Profiles[health.Name]
	health.Type, health.Note = profile.Type, profile.Note
	health.Countries, health.BlockedCountries = emptyIfNil(profile.Countries), emptyIfNil(profile.BlockedCountries)
	health.FeedGroup = s.cfg.Feed(health.Name)
	health.Feed = feedPoll
	if factory, ok := sources.Lookup(health.Name); ok {
		health.Feed = feedOf(factory, s.cfg.Sources[health.Name])
	}
	health.Coverage = SourceCoverage{Leagues: emptyIfNil(coverage.Leagues), Markets: emptyIfNil(coverage.Markets)}
}

func feedOf(factory sources.Factory, cfg sources.Config) string {
	if _, ok := factory(sources.Deps{Config: cfg}).(sources.Streamer); ok {
		return feedStream
	}
	return feedPoll
}

func (s *Server) sourceNames() []string {
	return slices.DeleteFunc(sources.Names(), func(name string) bool { return name == fakeSourceName })
}

func sourceHealthFromHash(name string, enabled bool, hash map[string]string) SourceHealth {
	health := SourceHealth{
		Name:                name,
		Enabled:             enabled,
		Degraded:            hash["degraded"] == "1",
		LastError:           errorLabel(hash["lastError"]),
		ConsecutiveFailures: int64Field(hash, "consecutiveFailures"),
		Requests:            int64Field(hash, "requests"),
		Errors:              int64Field(hash, "errors"),
		LastFetchMs:         int64Field(hash, "lastFetchMs"),
		FreshnessMs:         int64Field(hash, "freshnessMs"),
	}
	if health.Requests > 0 {
		health.ErrorRate = float64(health.Errors) / float64(health.Requests)
	}
	if lastSuccess, err := time.Parse(time.RFC3339Nano, hash["lastSuccess"]); err == nil {
		health.LastSuccess = &lastSuccess
	}
	return health
}

const (
	errorBlocked     = "Blocked from this region"
	errorRateLimited = "Rate limited"
	errorTimedOut    = "Timed out"
	errorNoOdds      = "No odds returned"
	errorConnection  = "Connection failed"
	errorUpstream    = "Upstream error"
)

var errorLabelMarkers = []struct {
	label   string
	markers []string
}{
	{errorBlocked, []string{"status 401", "status 403", "status 451", "captcha", "cloudflare", "blocked"}},
	{errorRateLimited, []string{"status 429", "rate limit", "throttled", "too many requests"}},
	{errorNoOdds, []string{"no odds", "empty body", "no events", "no markets"}},
	{errorTimedOut, []string{"deadline exceeded", "timeout", "timed out"}},
	{errorConnection, []string{"connection refused", "connection reset", "no such host", "network is unreachable", "tls", "eof"}},
}

func errorLabel(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	for _, l := range errorLabelMarkers {
		if slices.ContainsFunc(l.markers, func(marker string) bool { return strings.Contains(lower, marker) }) {
			return l.label
		}
	}
	return errorUpstream
}

func int64Field(hash map[string]string, field string) int64 {
	n, err := strconv.ParseInt(hash[field], 10, 64)
	if err != nil {
		return 0
	}
	return n
}
