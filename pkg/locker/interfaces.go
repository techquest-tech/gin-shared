package locker

import (
	"context"
	"errors"
	"time"
)

var ErrLocked = errors.New("locker: resource locked")

type Locker interface {
	Lock(ctx context.Context, resource string) (Release, error)
	WaitForLocker(ctx context.Context, resource string, maxWait time.Duration, timeout time.Duration) (Release, error)
	LockWithtimeout(ctx context.Context, resource string, timeout time.Duration) (Release, error)
}

type Release func(context.Context) error
