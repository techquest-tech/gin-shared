//go:build ram

package locker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

// var glocker sync.Mutex

type LocalLocker struct {
	locker sync.Map
	// ticker time.Duration
}

func (ml *LocalLocker) Lock(ctx context.Context, resource string) (Release, error) {
	logger := zap.L().With(zap.String("resources", resource))
	var locker *sync.Mutex
	raw, ok := ml.locker.Load(resource)
	if !ok {
		locker = &sync.Mutex{}
		ml.locker.Store(resource, locker)
	} else {
		locker = raw.(*sync.Mutex)
	}

	ok = locker.TryLock()
	if ok {
		logger.Debug("get locker done")
		release := func(ctx context.Context) error {
			locker.Unlock()
			return nil
		}
		return release, nil
	}
	logger.Info("get locker failed")
	return nil, errors.New(resource + " is locked")

	// locker.Lock()
	// return func(ctx context.Context) error {
	// 	locker.Unlock()
	// 	return nil
	// }, nil
}
func (ml *LocalLocker) WaitForLocker(ctx context.Context, resource string, maxWait time.Duration, timeout time.Duration) (Release, error) {
	if timeout > 0 || maxWait > 0 {
		// ml.ticker = timeout
		zap.L().Debug("timeout or max wait is not supported in ram locker")
	}
	return ml.Lock(ctx, resource)
}

func (ml *LocalLocker) LockWithtimeout(ctx context.Context, resource string, timeout time.Duration) (Release, error) {
	if timeout > 0 {
		// ml.ticker = timeout
		zap.L().Debug("timeout is not supported in ram locker")
	}
	return ml.Lock(ctx, resource)
	// logger := zap.L().With(zap.String("resources", resource))
	// var locker *sync.Mutex
	// raw, ok := ml.locker.Load(resource)
	// if !ok {
	// 	locker = &sync.Mutex{}
	// 	ml.locker.Store(resource, locker)
	// } else {
	// 	locker = raw.(*sync.Mutex)
	// }

	// start := time.Now()
	// for {
	// 	logger.Debug("waiting for locker")
	// 	ok := locker.TryLock()
	// 	if ok {
	// 		logger.Debug("done")
	// 		return func(ctx context.Context) error {
	// 			locker.Unlock()
	// 			return nil
	// 		}, nil
	// 	}
	// 	time.Sleep(ml.ticker)
	// 	dur := time.Since(start)
	// 	if dur > timeout {
	// 		logger.Warn("get locker failed.")
	// 		return nil, errors.New("timeout")
	// 	}
	// }
}

func InitLocalLocker() Locker {
	return &LocalLocker{
		locker: sync.Map{},
		// ticker: time.Second,
	}
}

func init() {
	core.Provide(InitLocalLocker)
}
