package messaging

import (
	"context"
	"strconv"
	"strings"
	"time"
)

type ctxKey int

const messageIDKey ctxKey = iota

func WithMessageID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, messageIDKey, id)
}

func MessageIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v := ctx.Value(messageIDKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func StreamIDTime(id string) (time.Time, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return time.Time{}, false
	}
	ts := id
	if idx := strings.IndexByte(id, '-'); idx > 0 {
		ts = id[:idx]
	}
	ms, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}, false
	}
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond)), true
}

