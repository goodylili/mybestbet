package browserd

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"mybestbet/pkg/sources"
)

const (
	navigationBudget       = 30 * time.Second
	minNavigationGap       = 2 * time.Second
	maxNavigationJitter    = 1500 * time.Millisecond
	bodyReadBudget         = 10 * time.Second
	maxCapturedBodyPerItem = 8 << 20
)

func fetchWithProfile(ctx context.Context, prof *profile, req sources.BrowserRequest) (sources.BrowserResult, error) {
	wait := time.Duration(req.WaitMillis) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, navigationBudget+wait+bodyReadBudget)
	defer cancel()

	prof.mu.Lock()
	defer prof.mu.Unlock()

	page, isReused, err := prof.pageFor(req.URL, time.Now())
	if err != nil {
		return sources.BrowserResult{}, err
	}
	if err := pauseLikeAHuman(ctx, prof.lastNavigation); err != nil {
		return sources.BrowserResult{}, err
	}
	page = page.Context(ctx)
	capture, err := captureWhileLoading(ctx, page, req, isReused, wait)
	prof.lastNavigation = time.Now()
	if err != nil {
		prof.dropPage(req.URL)
		return sources.BrowserResult{}, err
	}
	return readCapturedResult(page, capture, prof.userAgent)
}

func pauseLikeAHuman(ctx context.Context, lastNavigation time.Time) error {
	jitter := time.Duration(rand.Int64N(int64(maxNavigationJitter))) //nolint:gosec
	pause := max(time.Until(lastNavigation.Add(minNavigationGap)), 0) + jitter
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("pace navigation: %w", ctx.Err())
	}
}

func captureWhileLoading(ctx context.Context, page *rod.Page, req sources.BrowserRequest, isReused bool, wait time.Duration) (*collector, error) {
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		return nil, fmt.Errorf("enable network domain: %w", err)
	}
	capture := newCollector(req)
	listenCtx, stopListening := context.WithCancel(ctx)
	listen := page.Context(listenCtx).EachEvent(
		capture.onRequestWillBeSent,
		capture.onResponseReceived,
		capture.onLoadingFinished,
		capture.onWebSocketCreated,
		capture.onWebSocketFrame,
	)
	listening := make(chan struct{})
	go func() {
		defer close(listening)
		listen()
	}()
	defer func() {
		stopListening()
		<-listening
	}()

	if err := loadPage(page, req.URL, isReused); err != nil {
		return nil, err
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return capture, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for page traffic: %w", ctx.Err())
	}
}

func loadPage(page *rod.Page, url string, isReused bool) error {
	if isReused {
		if err := page.Reload(); err != nil {
			return fmt.Errorf("reload page: %w", err)
		}
		return nil
	}
	if err := page.Navigate(url); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	return nil
}

func readCapturedResult(page *rod.Page, capture *collector, userAgent string) (sources.BrowserResult, error) {
	result := sources.BrowserResult{Headers: capture.forwardedHeaders(), Cookies: map[string]string{}}
	if _, ok := result.Headers["User-Agent"]; !ok {
		result.Headers["User-Agent"] = userAgent
	}
	capturedBytes := 0
	for _, resp := range capture.finishedResponses() {
		body, err := responseBody(page, resp.requestID)
		if err != nil || len(body) > maxCapturedBodyPerItem || capturedBytes+len(body) > maxCapturedBytes {
			continue
		}
		capturedBytes += len(body)
		result.Captured = append(result.Captured, sources.Captured{URL: resp.url, Status: resp.status, Body: body})
	}
	result.Captured = append(result.Captured, capture.capturedFrames()...)
	cookies, err := page.Cookies(nil)
	if err != nil {
		return result, fmt.Errorf("read cookies: %w", err)
	}
	for _, c := range cookies {
		result.Cookies[c.Name] = c.Value
	}
	return result, nil
}

func responseBody(page *rod.Page, requestID proto.NetworkRequestID) ([]byte, error) {
	res, err := proto.NetworkGetResponseBody{RequestID: requestID}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if !res.Base64Encoded {
		return []byte(res.Body), nil
	}
	body, err := base64.StdEncoding.DecodeString(res.Body)
	if err != nil {
		return nil, fmt.Errorf("decode response body: %w", err)
	}
	return body, nil
}
