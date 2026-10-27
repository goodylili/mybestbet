package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestPendingUpdateKeepsLatestScore(t *testing.T) {
	t.Parallel()
	pending := newPendingUpdate("e1")
	pending.merge(canon.BoardUpdate{EventID: "e1", Score: &canon.EventScore{Home: 1, Minute: 10}})
	pending.merge(canon.BoardUpdate{EventID: "e1", Score: &canon.EventScore{Home: 2, Minute: 30}})
	pending.merge(canon.BoardUpdate{EventID: "e1"})
	assert.False(t, pending.isEmpty())

	now := time.Now()
	assert.Equal(t, canon.BoardUpdate{EventID: "e1", Score: &canon.EventScore{Home: 2, Minute: 30}, SentAt: now}, pending.drain(now))
	assert.True(t, pending.isEmpty())
}
