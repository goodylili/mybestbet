package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogoCaching(t *testing.T) {
	pool, rdb := integrationDeps(t)
	f := seedContractFixture(t, pool, rdb)
	s := newTestServer(t, pool, rdb, f)
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)
	token := userToken(t, s, pool)
	etag := `"` + contractLogoSHA + `"`
	cases := []struct {
		name        string
		path        string
		ifNoneMatch string
		status      int
		cache       string
		body        string
	}{
		{name: "current version is immutable", path: "/v1/logos/source/bet9ja" + contractLogoURL, status: http.StatusOK, cache: immutableLogoCache, body: contractLogoSVG},
		{name: "stale version revalidates", path: "/v1/logos/source/bet9ja?v=old", status: http.StatusOK, cache: revalidatedLogoCache, body: contractLogoSVG},
		{name: "matching etag", path: "/v1/logos/league/NBA" + contractLogoURL, ifNoneMatch: etag, status: http.StatusNotModified, cache: immutableLogoCache},
		{name: "weak etag in list", path: "/v1/logos/team/" + f.homeTeamID, ifNoneMatch: `"other", W/` + etag, status: http.StatusNotModified, cache: revalidatedLogoCache},
		{name: "different etag", path: "/v1/logos/team/" + f.homeTeamID, ifNoneMatch: `"other"`, status: http.StatusOK, cache: revalidatedLogoCache, body: contractLogoSVG},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL+tc.path, nil)
			require.NoError(t, err)
			authorize(req, token)
			if tc.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tc.ifNoneMatch)
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.status, resp.StatusCode)
			assert.Equal(t, etag, resp.Header.Get("ETag"))
			assert.Equal(t, tc.cache, resp.Header.Get("Cache-Control"))
			assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
			assert.Equal(t, svgContentSecurityPolicy, resp.Header.Get("Content-Security-Policy"))
			assert.Equal(t, tc.body, string(raw))
			if tc.status == http.StatusOK {
				assert.Equal(t, "image/svg+xml", resp.Header.Get("Content-Type"))
			}
		})
	}
}

func TestGetLogoMissingIsNosniffJSON(t *testing.T) {
	pool, rdb := integrationDeps(t)
	s := newTestServer(t, pool, rdb, contractFixture{})
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)
	req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/logos/league/NO_SUCH_LEAGUE", nil)
	require.NoError(t, err)
	authorize(req, userToken(t, s, pool))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Empty(t, resp.Header.Get("ETag"))
}

func TestETagMatches(t *testing.T) {
	t.Parallel()
	assert.True(t, etagMatches(`"a"`, `"a"`))
	assert.True(t, etagMatches(`*`, `"a"`))
	assert.True(t, etagMatches(`"b" , W/"a"`, `"a"`))
	assert.False(t, etagMatches(``, `"a"`))
	assert.False(t, etagMatches(`"ab"`, `"a"`))
}
