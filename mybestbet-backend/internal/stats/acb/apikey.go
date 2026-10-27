package acb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/goccy/go-json"

	"mybestbet/pkg/sources"
)

const apiKeyHeader = "X-APIKEY"

var (
	ErrAPIKeyNotFound = errors.New("acb: api key not found in site bundle")
	errStaleAPIKey    = errors.New("acb: api key rejected")
	siteScriptChunk   = regexp.MustCompile(`"(/_next/static/chunks/[A-Za-z0-9_.-]+\.js)`)
	apiKeyInBundle    = regexp.MustCompile(`"X-APIKEY":"([0-9A-Fa-f-]{36})"`)
)

func (c *Client) getAPI(ctx context.Context, path string, out any) (hasContent bool, err error) {
	body, err := c.getWithAPIKey(ctx, path)
	if errors.Is(err, errStaleAPIKey) {
		body, err = c.getWithAPIKey(ctx, path)
	}
	if err != nil {
		return false, fmt.Errorf("acb %s: %w", path, err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || bytes.Equal(body, []byte("null")) {
		return false, nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, fmt.Errorf("decode acb %s: %w", path, err)
	}
	return true, nil
}

func (c *Client) getWithAPIKey(ctx context.Context, path string) ([]byte, error) {
	key, err := c.currentAPIKey(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set(apiKeyHeader, key)
	body, err := c.deps.Do(req)
	if errors.Is(err, sources.ErrForbidden) {
		c.forgetAPIKey(key)
		return nil, errors.Join(errStaleAPIKey, err)
	}
	return body, err
}

func (c *Client) currentAPIKey(ctx context.Context) (string, error) {
	c.apiKeyMu.Lock()
	defer c.apiKeyMu.Unlock()
	if c.apiKey != "" {
		return c.apiKey, nil
	}
	key, err := c.discoverAPIKey(ctx)
	if err != nil {
		return "", err
	}
	c.apiKey = key
	return key, nil
}

func (c *Client) forgetAPIKey(rejected string) {
	c.apiKeyMu.Lock()
	defer c.apiKeyMu.Unlock()
	if c.apiKey == rejected {
		c.apiKey = ""
	}
}

func (c *Client) discoverAPIKey(ctx context.Context) (string, error) {
	page, err := c.getSiteFile(ctx, c.keyPageURL)
	if err != nil {
		return "", fmt.Errorf("acb key page: %w", err)
	}
	site, err := url.Parse(c.keyPageURL)
	if err != nil {
		return "", fmt.Errorf("acb key page url: %w", err)
	}
	var chunkErrs []error
	for _, chunk := range siteScriptChunk.FindAllSubmatch(page, -1) {
		chunkURL := site.Scheme + "://" + site.Host + string(chunk[1])
		js, err := c.getSiteFile(ctx, chunkURL)
		if err != nil {
			chunkErrs = append(chunkErrs, err)
			continue
		}
		if found := apiKeyInBundle.FindSubmatch(js); found != nil {
			return string(found[1]), nil
		}
	}
	return "", errors.Join(append([]error{ErrAPIKeyNotFound}, chunkErrs...)...)
}

func (c *Client) getSiteFile(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	return c.deps.Do(req)
}
