package notify

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramClientSendsToTokenPath(t *testing.T) {
	t.Parallel()
	fake := &fakeTelegram{updates: []telegramUpdate{{UpdateID: 7, Message: &telegramMessage{Chat: telegramChat{ID: 99}, Text: "/start"}}}}
	srv := fake.serve(t)
	client := &telegramClient{baseURL: srv.URL, token: testToken, http: srv.Client()}

	updates, err := client.updates(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, int64(7), updates[0].UpdateID)
	assert.Equal(t, int64(99), updates[0].Message.Chat.ID)

	require.NoError(t, client.sendMessage(context.Background(), 99, "hi"))
	assert.Equal(t, []sentMessage{{ChatID: 99, Text: "hi"}}, fake.messages())
	assert.Equal(t, "/bot"+testToken+"/sendMessage", fake.paths[1])
}

func TestTelegramClientErrorsNeverLeakToken(t *testing.T) {
	t.Parallel()
	fake := &fakeTelegram{fail: true}
	srv := fake.serve(t)
	client := &telegramClient{baseURL: srv.URL, token: testToken, http: srv.Client()}
	err := client.sendMessage(context.Background(), 1, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Too Many Requests")
	assert.NotContains(t, err.Error(), testToken)

	unreachable := &telegramClient{baseURL: "http://127.0.0.1:1", token: testToken, http: &http.Client{}}
	err = unreachable.sendMessage(context.Background(), 1, "x")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testToken)
}
