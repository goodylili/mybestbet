package loadtest

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
)

const (
	maxSSELineBytes  = 64 << 20
	sseDataPrefix    = "data: "
	sseConnectedLine = ": connected"
)

var errStreamClosed = errors.New("loadtest: sse stream closed before it connected")

type sseStream struct {
	body    io.Closer
	scanner *bufio.Scanner
}

func openSSEStream(ctx context.Context, client *http.Client, apiURL, eventID string) (*sseStream, error) {
	endpoint := strings.TrimRight(apiURL, "/") + "/v1/stream?event=" + url.QueryEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build sse request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open sse stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("open sse stream: status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxSSELineBytes)
	for scanner.Scan() {
		if scanner.Text() == sseConnectedLine {
			return &sseStream{body: resp.Body, scanner: scanner}, nil
		}
	}
	_ = resp.Body.Close()
	return nil, errors.Join(errStreamClosed, scanner.Err())
}

func observeSSE(ctx context.Context, stream *sseStream, t *tracker) error {
	defer func() { _ = stream.body.Close() }()
	for stream.scanner.Scan() {
		line := stream.scanner.Text()
		if !strings.HasPrefix(line, sseDataPrefix) {
			continue
		}
		receivedAt := time.Now()
		var update canon.BoardUpdate
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, sseDataPrefix)), &update); err != nil {
			return fmt.Errorf("decode sse update: %w", err)
		}
		t.observeUpdate(update, receivedAt)
	}
	if err := stream.scanner.Err(); err != nil && !isShutdown(ctx, err) {
		return fmt.Errorf("read sse stream: %w", err)
	}
	if ctx.Err() == nil {
		return fmt.Errorf("sse stream for %s ended early (client lagging?)", t.path)
	}
	return nil
}
