package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	json "github.com/goccy/go-json"
)

const (
	DefaultTelegramBaseURL = "https://api.telegram.org"
	longPollWait           = 25 * time.Second
	longPollSlack          = 10 * time.Second
	sendTimeout            = 10 * time.Second
	maxTelegramBodyBytes   = 4 << 20
)

type telegramClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func (c *telegramClient) updates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	ctx, cancel := context.WithTimeout(ctx, longPollWait+longPollSlack)
	defer cancel()
	request := getUpdatesRequest{Offset: offset, Timeout: int(longPollWait.Seconds()), AllowedUpdates: []string{"message"}}
	return callTelegram[[]telegramUpdate](ctx, c, "getUpdates", request)
}

func (c *telegramClient) sendMessage(ctx context.Context, chatID int64, html string) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	request := sendMessageRequest{ChatID: chatID, Text: html, ParseMode: "HTML", DisableWebPagePreview: true}
	_, err := callTelegram[json.RawMessage](ctx, c, "sendMessage", request)
	return err
}

func callTelegram[T any](ctx context.Context, c *telegramClient, method string, payload any) (T, error) {
	var zero T
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, fmt.Errorf("telegram %s: encode: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("telegram %s: build request: %w", method, redactURL(err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return zero, fmt.Errorf("telegram %s: %w", method, redactURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramBodyBytes))
	if err != nil {
		return zero, fmt.Errorf("telegram %s: read body: %w", method, err)
	}
	var envelope telegramResponse[T]
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return zero, fmt.Errorf("telegram %s: status %d: decode: %w", method, resp.StatusCode, err)
	}
	if !envelope.OK {
		return zero, fmt.Errorf("telegram %s: status %d: %s", method, resp.StatusCode, envelope.Description)
	}
	return envelope.Result, nil
}

func redactURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
