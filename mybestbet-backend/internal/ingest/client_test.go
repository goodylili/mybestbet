package ingest

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/sources"
)

func compressed(t *testing.T, encoding string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w io.WriteCloser
	switch encoding {
	case "gzip":
		w = gzip.NewWriter(&buf)
	case "br":
		w = brotli.NewWriter(&buf)
	default:
		return body
	}
	_, err := w.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestSourceClientDecodesGzipAndBrotli(t *testing.T) {
	payload := []byte(`{"events":[1,2,3]}`)
	for _, encoding := range []string{"gzip", "br", ""} {
		t.Run("encoding "+encoding, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, acceptedEncodings, r.Header.Get("Accept-Encoding"))
				if encoding != "" {
					w.Header().Set("Content-Encoding", encoding)
				}
				_, _ = w.Write(compressed(t, encoding, payload))
			}))
			defer srv.Close()
			client, err := newSourceHTTPClient(sources.Config{})
			require.NoError(t, err)
			deps := sources.Deps{HTTP: client}
			body, err := deps.Get(t.Context(), srv.URL)
			require.NoError(t, err)
			assert.Equal(t, payload, body)
		})
	}
}

func TestSourceClientRejectsBadProxy(t *testing.T) {
	_, err := newSourceHTTPClient(sources.Config{Proxy: "://bad"})
	assert.Error(t, err)
}

func TestSourceClientCountsRequestsOfACountedContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client, err := newSourceHTTPClient(sources.Config{})
	require.NoError(t, err)
	deps := sources.Deps{HTTP: client}
	var sent atomic.Int64
	counted := withRequestCounter(t.Context(), &sent)
	for range 3 {
		_, err := deps.Get(counted, srv.URL)
		require.NoError(t, err)
	}
	_, err = deps.Get(t.Context(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, int64(3), sent.Load())
}
