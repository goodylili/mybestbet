package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestActiveFamilies(t *testing.T) {
	t.Parallel()
	now := time.Now()
	var a activeFamilies
	assert.False(t, a.has("f1", now))
	a.remember("f1", now)
	assert.True(t, a.has("f1", now.Add(localActiveTTL-time.Millisecond)))
	assert.False(t, a.has("f1", now.Add(localActiveTTL)), "an entry lives for the local ttl only")
	a.forget("f1")
	assert.False(t, a.has("f1", now), "a revoked family is forgotten at once")
	for i := range maxLocalActiveFamily {
		a.until[string(rune(i))] = now
	}
	a.remember("f2", now)
	assert.LessOrEqual(t, len(a.until), maxLocalActiveFamily)
	assert.True(t, a.has("f2", now))
}
