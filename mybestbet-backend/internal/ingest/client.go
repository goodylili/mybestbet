package ingest

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"

	"mybestbet/pkg/sources"
)

const (
	dialTimeout         = 5 * time.Second
	tcpKeepAlive        = 30 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
	idleConnTimeout     = 90 * time.Second
	maxIdleConns        = 64
	maxIdleConnsPerHost = 16
	acceptedEncodings   = "gzip, br"
)

type requestCounterKey struct{}

func withRequestCounter(ctx context.Context, sent *atomic.Int64) context.Context {
	return context.WithValue(ctx, requestCounterKey{}, sent)
}

func newSourceHTTPClient(cfg sources.Config) (*http.Client, error) {
	transport, err := newPooledTransport(cfg.Proxy)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: decompressingTransport{base: transport}}, nil
}

func newPooledTransport(proxy string) (*http.Transport, error) {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: dialTimeout, KeepAlive: tcpKeepAlive}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeout,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
		DisableCompression:  true,
	}
	if proxy == "" {
		return transport, nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse source proxy: %w", err)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return transport, nil
}

type decompressingTransport struct {
	base http.RoundTripper
}

func (t decompressingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if sent, ok := req.Context().Value(requestCounterKey{}).(*atomic.Int64); ok {
		sent.Add(1)
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Accept-Encoding", acceptedEncodings)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	return decodeBody(resp)
}

func decodeBody(resp *http.Response) (*http.Response, error) {
	var decoded io.Reader
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("open gzip body: %w", err)
		}
		decoded = gz
	case "br":
		decoded = brotli.NewReader(resp.Body)
	default:
		return resp, nil
	}
	resp.Body = decodedBody{Reader: decoded, compressed: resp.Body}
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.ContentLength = -1
	resp.Uncompressed = true
	return resp, nil
}

type decodedBody struct {
	io.Reader
	compressed io.Closer
}

func (b decodedBody) Close() error { return b.compressed.Close() }
