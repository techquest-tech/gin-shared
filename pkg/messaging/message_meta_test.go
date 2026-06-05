package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMessageIDFromContext(t *testing.T) {
	ctx := WithMessageID(context.Background(), "1710000000000-0")
	id, ok := MessageIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "1710000000000-0", id)
}

func TestStreamIDTime(t *testing.T) {
	tt, ok := StreamIDTime("1710000000000-0")
	assert.True(t, ok)
	assert.Equal(t, time.Unix(1710000000000/1000, 0), tt)

	_, ok = StreamIDTime("")
	assert.False(t, ok)

	_, ok = StreamIDTime("bad")
	assert.False(t, ok)
}

