package logos

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encoded(t *testing.T, encode func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))))
	return buf.Bytes()
}

func pngBytes(t *testing.T) []byte {
	return encoded(t, func(b *bytes.Buffer, m image.Image) error { return png.Encode(b, m) })
}

func TestSniffImage(t *testing.T) {
	t.Parallel()
	webp := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 24)...)
	cases := []struct {
		name string
		body []byte
		want string
		ok   bool
	}{
		{name: "png", body: pngBytes(t), want: "image/png", ok: true},
		{name: "jpeg", body: encoded(t, func(b *bytes.Buffer, m image.Image) error { return jpeg.Encode(b, m, nil) }), want: "image/jpeg", ok: true},
		{name: "gif", body: encoded(t, func(b *bytes.Buffer, m image.Image) error { return gif.Encode(b, m, nil) }), want: "image/gif", ok: true},
		{name: "webp", body: webp, want: "image/webp", ok: true},
		{name: "bare svg", body: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`), want: contentTypeSVG, ok: true},
		{name: "svg with prologue", body: []byte("\xef\xbb\xbf<?xml version=\"1.0\"?>\n<!-- logo -->\n<!DOCTYPE svg>\n<SVG width=\"1\">"), want: contentTypeSVG, ok: true},
		{name: "html page", body: []byte("<!doctype html><html><body><svg></svg></body></html>"), ok: false},
		{name: "json error", body: []byte(`{"error":"not found"}`), ok: false},
		{name: "ico", body: []byte{0, 0, 1, 0, 1, 0, 16, 16, 0, 0, 1, 0, 32, 0}, ok: false},
		{name: "empty", body: nil, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := sniffImage(tc.body)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsPublic(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "::1": false,
		"fe80::1": false, "fc00::1": false, "224.0.0.1": false, "198.18.0.1": false,
		"8.8.8.8": true, "151.101.1.69": true, "2606:4700::1111": true,
	}
	for address, want := range cases {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, isPublic(netip.MustParseAddr(address).Unmap()))
		})
	}
}

func TestRefuseNonPublicMappedIPv4(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, refuseNonPublic("tcp6", "[::ffff:127.0.0.1]:443", nil), ErrBlockedAddress)
	require.NoError(t, refuseNonPublic("tcp4", "8.8.8.8:443", nil))
}

func TestFetchRefusesLoopbackServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(pngBytes(t)) }))
	t.Cleanup(srv.Close)
	_, err := NewFetcher().Fetch(t.Context(), srv.URL+"/logo.png")
	require.ErrorIs(t, err, ErrBlockedAddress)
}

func TestFetchRefusesRedirectToLoopback(t *testing.T) {
	t.Parallel()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(pngBytes(t)) }))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(redirector.Close)
	guardCalls := 0
	fetcher := newFetcher(func(network, address string, _ syscall.RawConn) error {
		guardCalls++
		if guardCalls > 1 {
			return refuseNonPublic(network, address, nil)
		}
		return nil
	})
	_, err := fetcher.Fetch(t.Context(), redirector.URL)
	require.ErrorIs(t, err, ErrBlockedAddress)
}

func TestFetchLimits(t *testing.T) {
	t.Parallel()
	oversized := append(pngBytes(t), make([]byte, MaxImageBytes)...)
	cases := []struct {
		name    string
		url     string
		handler http.HandlerFunc
		wantErr error
		want    string
	}{
		{name: "png accepted despite wrong header", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(pngBytes(t))
		}, want: "image/png"},
		{name: "html with image header rejected", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<html><body>blocked</body></html>"))
		}, wantErr: ErrNotImage},
		{name: "declared length over limit", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(len(oversized)))
			_, _ = w.Write(oversized)
		}, wantErr: ErrTooLarge},
		{name: "streamed body over limit", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = w.Write(oversized)
		}, wantErr: ErrTooLarge},
		{name: "upstream 404", handler: func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }, wantErr: ErrUpstreamStatus},
		{name: "non http scheme", url: "file:///etc/passwd", wantErr: ErrUnsupportedURL},
		{name: "credentials in url", url: "https://user:pass@example.com/logo.png", wantErr: ErrUnsupportedURL},
		{name: "relative url", url: "/logo.png", wantErr: ErrUnsupportedURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			target := tc.url
			if tc.handler != nil {
				srv := httptest.NewServer(tc.handler)
				t.Cleanup(srv.Close)
				target = srv.URL + "/logo"
			}
			image, err := newFetcher(nil).Fetch(t.Context(), target)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, image.contentType)
			assert.Len(t, image.sha256, 64)
		})
	}
}

func TestURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/v1/logos/source/22bet?v=abc", URL(KindSource, "22bet", "abc"))
	assert.Equal(t, "/v1/logos/league/A%2FB?v=abc", URL(KindLeague, "A/B", "abc"))
	assert.Empty(t, URL(KindTeam, "x", ""))
	assert.Equal(t, "0123456789ab", Logo{SHA256: strings.Repeat("0123456789abcdef", 4)}.Version())
}
