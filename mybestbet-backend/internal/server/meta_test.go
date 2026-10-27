package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTelegramBotURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		username string
		want     string
	}{
		{name: "unset", username: "", want: ""},
		{name: "plain", username: "mybestbet_bot", want: "https://t.me/mybestbet_bot"},
		{name: "at prefixed and padded", username: " @mybestbet_bot ", want: "https://t.me/mybestbet_bot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, telegramBotURL(tt.username))
		})
	}
}

func TestBuildVersionIsNeverEmpty(t *testing.T) {
	t.Parallel()
	assert.NotEmpty(t, buildVersion())
}

func TestCountryCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "ng", want: "NG"},
		{raw: " ID ", want: "ID"},
		{raw: "XX", want: ""},
		{raw: "T1", want: ""},
		{raw: "USA", want: ""},
		{raw: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, countryCode(tt.raw))
		})
	}
}

func TestLookupCountryWithoutDatabase(t *testing.T) {
	t.Parallel()
	assert.Empty(t, lookupCountry(nil, "8.8.8.8"))
	assert.Nil(t, openCountryDB("", nil))
}
