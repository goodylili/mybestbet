package auth

import (
	"bytes"
	"context"

	json "github.com/goccy/go-json"
)

const MaxSettingsBytes = 16 << 10

func (s *Service) Settings(ctx context.Context, userID string) (json.RawMessage, error) {
	return userSettings(ctx, s.pool, userID)
}

func (s *Service) PutSettings(ctx context.Context, userID string, settings json.RawMessage) (json.RawMessage, error) {
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	return replaceSettings(ctx, s.pool, userID, settings)
}

func validateSettings(settings json.RawMessage) error {
	if len(settings) > MaxSettingsBytes {
		return invalidInput("settings", "must be at most 16 KiB")
	}
	trimmed := bytes.TrimSpace(settings)
	var object map[string]any
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &object) != nil {
		return invalidInput("settings", "must be a json object")
	}
	return nil
}
