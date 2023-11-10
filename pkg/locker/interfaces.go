package locker

import "context"

type Locker interface {
	Lock(ctx context.Context, resource string) (Release, error)
}

type Release func(context.Context) error
