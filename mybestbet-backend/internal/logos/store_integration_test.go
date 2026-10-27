package logos

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return NewStore(pool), pool
}

func isDue(t *testing.T, store *Store, now time.Time, entityID string) bool {
	t.Helper()
	due, err := store.due(t.Context(), now.Add(-StaleAfter), now.Add(-FailureBackoff), 100000)
	require.NoError(t, err)
	return slices.ContainsFunc(due, func(c candidate) bool { return c.EntityID == entityID })
}

func TestCandidateUpsertAndFetchAreIdempotent(t *testing.T) {
	store, pool := openTestStore(t)
	ctx := t.Context()
	body := pngBytes(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/broken.png" {
			http.Error(w, "gone", http.StatusGone)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	entityID := "test-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(t.Context(), `DELETE FROM logos WHERE "entityId" = $1`, entityID)
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	job := NewJob(JobDeps{Store: store, Fetcher: newFetcher(nil), Log: slog.Default(), Now: func() time.Time { return now }})

	for range 2 {
		require.NoError(t, store.UpsertCandidate(ctx, pool, KindTeam, entityID, srv.URL+"/crest.png"))
	}
	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM logos WHERE "entityId" = $1`, entityID).Scan(&rows))
	require.Equal(t, 1, rows)
	require.True(t, isDue(t, store, now, entityID))
	_, err := store.Get(ctx, KindTeam, entityID)
	require.ErrorIs(t, err, ErrNotFound)

	crest := candidate{Kind: KindTeam, EntityID: entityID, SourceURL: srv.URL + "/crest.png"}
	var first PassResult
	require.NoError(t, job.fetchAll(ctx, []candidate{crest}, &first))
	assert.Equal(t, PassResult{Stored: 1}, first)
	logo, err := store.Get(ctx, KindTeam, entityID)
	require.NoError(t, err)
	assert.Equal(t, "image/png", logo.ContentType)
	assert.Equal(t, body, logo.Bytes)
	versions, err := store.Versions(ctx, KindTeam, []string{entityID, "missing"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{entityID: logo.Version()}, versions)
	assert.False(t, isDue(t, store, now, entityID))

	var updatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT "updatedAt" FROM logos WHERE "entityId" = $1`, entityID).Scan(&updatedAt))
	var second PassResult
	require.NoError(t, job.fetchAll(ctx, []candidate{crest}, &second))
	assert.Equal(t, PassResult{Unchanged: 1}, second)
	var updatedAgain time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT "updatedAt" FROM logos WHERE "entityId" = $1`, entityID).Scan(&updatedAgain))
	assert.Equal(t, updatedAt, updatedAgain)

	require.NoError(t, store.UpsertCandidate(ctx, pool, KindTeam, entityID, srv.URL+"/crest.png"))
	_, err = store.Get(ctx, KindTeam, entityID)
	require.NoError(t, err, "repeating the same candidate must keep the stored logo")

	require.NoError(t, store.UpsertCandidate(ctx, pool, KindTeam, entityID, "https://other-cdn.example/crest.png"))
	_, err = store.Get(ctx, KindTeam, entityID)
	require.NoError(t, err, "a different host never replaces a stored logo")

	broken := candidate{Kind: KindTeam, EntityID: entityID, SourceURL: srv.URL + "/broken.png"}
	require.NoError(t, store.UpsertCandidate(ctx, pool, KindTeam, entityID, broken.SourceURL))
	_, err = store.Get(ctx, KindTeam, entityID)
	require.ErrorIs(t, err, ErrNotFound, "a changed url on the same host clears the stored logo for refetch")
	var failed PassResult
	require.NoError(t, job.fetchAll(ctx, []candidate{broken}, &failed))
	assert.Equal(t, PassResult{Failed: 1}, failed)
	var lastError string
	require.NoError(t, pool.QueryRow(ctx, `SELECT "lastError" FROM logos WHERE "entityId" = $1 AND "failedAt" IS NOT NULL`, entityID).Scan(&lastError))
	assert.Contains(t, lastError, "410")
	assert.False(t, isDue(t, store, now, entityID), "failed logos back off")
	assert.True(t, isDue(t, store, now.Add(FailureBackoff+time.Minute), entityID))

	require.NoError(t, store.UpsertCandidate(ctx, pool, KindTeam, entityID, "https://other-cdn.example/crest.png"))
	var sourceURL string
	require.NoError(t, pool.QueryRow(ctx, `SELECT "sourceUrl" FROM logos WHERE "entityId" = $1`, entityID).Scan(&sourceURL))
	assert.Equal(t, "https://other-cdn.example/crest.png", sourceURL, "a logo without bytes accepts another source")
	assert.EqualValues(t, 3, hits.Load())
}

func TestRegisterSourcesIsIdempotent(t *testing.T) {
	store, pool := openTestStore(t)
	name := "test-source-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(t.Context(), `DELETE FROM logos WHERE kind = 'source' AND "entityId" = $1`, name)
	})
	job := NewJob(JobDeps{Store: store, SourceLogos: map[string]string{name: "https://example.com/apple-touch-icon.png"}, Log: slog.Default(), Now: time.Now})
	require.NoError(t, job.registerSources(t.Context()))
	require.NoError(t, job.registerSources(t.Context()))
	var rows int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM logos WHERE kind = 'source' AND "entityId" = $1`, name).Scan(&rows))
	assert.Equal(t, 1, rows)
}

func TestLoadSourceLogoURLs(t *testing.T) {
	t.Parallel()
	urls, err := LoadSourceLogoURLs("../../config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, urls["betway"])
	for name, raw := range urls {
		assert.Regexp(t, `^https://`, raw, name)
	}
}
