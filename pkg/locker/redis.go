//go:build !ram || !locker_db

package locker

import (
	"context"
	"errors"
	"time"

	"github.com/bsm/redislock"
	"github.com/redis/go-redis/v9"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

var MaxLockerDuration time.Duration = 30 * time.Minute

const (
	LockerPrefix = "_locker_"
)

type RedisLocker struct {
	client *redis.Client
	Logger *zap.Logger
}

// func (r *RedisLocker) Init() {
// 	c := redis.NewClient(r.RedisOptions)
// 	r.client = c
// 	r.Logger.Info("redis locker ready.", zap.String("redis", r.RedisOptions.Addr))
// }

func (r *RedisLocker) LockWithtimeout(ctx context.Context, resource string, timeout time.Duration) (Release, error) {
	locker := redislock.New(r.client)
	if timeout == 0 {
		timeout = MaxLockerDuration
	}
	lock, err := locker.Obtain(ctx, LockerPrefix+resource, timeout, nil)
	if err != nil {
		if err == redislock.ErrNotObtained {
			r.Logger.Info("resource is locked.", zap.Error(err), zap.String("resource", resource))
			return nil, errors.New(resource + " is locked.")
		}
		panic("unexpected error, " + err.Error())
	}
	return lock.Release, nil
}

func (r *RedisLocker) Lock(ctx context.Context, resource string) (Release, error) {
	return r.LockWithtimeout(ctx, resource, 0)
}

func InitRedisLocker(logger *zap.Logger, client *redis.Client) Locker {
	rd := &RedisLocker{
		Logger: logger,
		client: client,
	}
	return rd
}

func init() {
	core.Provide(InitRedisLocker)
}
