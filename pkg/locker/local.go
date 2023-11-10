package locker

import (
	"context"
	"sync"
)

var glocker sync.Mutex

type LocalLocker struct {
	locker map[string]*sync.Mutex
}

func (ml *LocalLocker) Lock(ctx context.Context, resource string) (Release, error) {
	glocker.Lock()
	defer glocker.Unlock()

	locker, ok := ml.locker[resource]
	if !ok {
		locker = &sync.Mutex{}
		ml.locker[resource] = locker
	}
	locker.Lock()
	return func(ctx context.Context) error {
		locker.Unlock()
		return nil
	}, nil
}

func InitLocalLocker() *LocalLocker {
	return &LocalLocker{
		locker: make(map[string]*sync.Mutex),
	}
}
