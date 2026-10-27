package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/sources"
)

const browserWaitMillis = 1500

var ErrNoCapturedBody = errors.New("fetch: browser captured no successful response")

type Fetcher struct {
	Deps    sources.Deps
	Profile string
}

func (f Fetcher) Body(ctx context.Context, target string) ([]byte, error) {
	body, err := f.Deps.Get(ctx, target)
	if err == nil || f.Deps.Browser == nil || !isBlockedDirectly(ctx, err) {
		return body, err
	}
	return f.viaBrowser(ctx, target)
}

func isBlockedDirectly(ctx context.Context, err error) bool {
	if errors.Is(err, sources.ErrForbidden) {
		return true
	}
	var status *sources.StatusError
	return !errors.As(err, &status) && ctx.Err() == nil
}

func (f Fetcher) JSON(ctx context.Context, target string, out any) error {
	body, err := f.Body(ctx, target)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", target, err)
	}
	return nil
}

func (f Fetcher) viaBrowser(ctx context.Context, target string) ([]byte, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", target, err)
	}
	result, err := f.Deps.Browser.Fetch(ctx, sources.BrowserRequest{
		Profile:     f.Profile,
		URL:         target,
		CapturePath: []string{parsed.Path},
		WaitMillis:  browserWaitMillis,
	})
	if err != nil {
		return nil, fmt.Errorf("browser fetch %s: %w", parsed.Path, err)
	}
	for _, captured := range result.Captured {
		if captured.Status == http.StatusOK && !captured.WS {
			return captured.Body, nil
		}
	}
	return nil, fmt.Errorf("%s: %w", parsed.Path, ErrNoCapturedBody)
}
