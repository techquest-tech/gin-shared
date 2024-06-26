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

type RedisLocker struct {
	Timeout time.Duration
	Maxtry  uint
	client  *redis.Client
	Logger  *zap.Logger
}

// func (r *RedisLocker) Init() {
// 	c := redis.NewClient(r.RedisOptions)
// 	r.client = c
// 	r.Logger.Info("redis locker ready.", zap.String("redis", r.RedisOptions.Addr))
// }

func (r *RedisLocker) LockWithtimeout(ctx context.Context, resource string, timeout time.Duration) (Release, error) {
	locker := redislock.New(r.client)
	cnt := uint(0)
	var opt *redislock.Options
	if timeout > 0 {
		opt = &redislock.Options{
			RetryStrategy: redislock.LinearBackoff(timeout),
		}
	}
	for {
		lock, err := locker.Obtain(ctx, resource, r.Timeout, opt)
		if err != nil {
			if err == redislock.ErrNotObtained {
				cnt += 1
				if cnt > r.Maxtry {
					return nil, errors.New("get locker failed after max try")
				}
				r.Logger.Debug("ErrNotObtained, try again", zap.Uint("cnt", cnt))
				time.Sleep(r.Timeout)
				continue
			}
			r.Logger.Error("get locker failed.", zap.Error(err), zap.String("resource", resource))
			return nil, err
		}
		return lock.Release, nil
	}
}

func (r *RedisLocker) Lock(ctx context.Context, resource string) (Release, error) {
	return r.LockWithtimeout(ctx, resource, 0)
}

func InitRedisLocker(logger *zap.Logger, client *redis.Client) Locker {
	rd := &RedisLocker{
		Timeout: time.Millisecond * 50,
		Logger:  logger,
		Maxtry:  9999,
		client:  client,
	}
	return rd
}

func init() {
	core.Provide(InitRedisLocker)
}
