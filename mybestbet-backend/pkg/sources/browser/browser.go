package browser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/sources"
)

const maxResultBytes = 64 << 20

type Client struct {
	fetchURL string
	http     *http.Client
}

func New(baseURL string, client *http.Client) sources.Browser {
	return &Client{fetchURL: strings.TrimRight(baseURL, "/") + "/fetch", http: client}
}

func (c *Client) Fetch(ctx context.Context, req sources.BrowserRequest) (sources.BrowserResult, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return sources.BrowserResult{}, fmt.Errorf("encode browser request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.fetchURL, bytes.NewReader(payload))
	if err != nil {
		return sources.BrowserResult{}, fmt.Errorf("build browser request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return sources.BrowserResult{}, fmt.Errorf("browserd fetch %s: %w", req.Profile, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return sources.BrowserResult{}, &sources.StatusError{Method: http.MethodPost, URL: c.fetchURL, Status: resp.StatusCode}
	}
	var result sources.BrowserResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResultBytes)).Decode(&result); err != nil {
		return sources.BrowserResult{}, fmt.Errorf("decode browserd result: %w", err)
	}
	return result, nil
}
