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

var MaxLockerDuration time.Duration = 30 * time.Second
var WaitInteval time.Duration = 50 * time.Millisecond

const (
	LockerPrefix = "_locker_"
)

type RedisLocker struct {
	client *redis.Client
	Logger *zap.Logger
}

//	func (r *RedisLocker) Init() {
//		c := redis.NewClient(r.RedisOptions)
//		r.client = c
//		r.Logger.Info("redis locker ready.", zap.String("redis", r.RedisOptions.Addr))
//	}
func (r *RedisLocker) WaitForLocker(ctx context.Context, resource string, maxWait time.Duration, timeout time.Duration) (Release, error) {
	ll := r.Logger.With(zap.String("resource", resource))
	locker := redislock.New(r.client)
	if timeout == 0 {
		timeout = MaxLockerDuration
	}
	opt := &redislock.Options{}

	if maxWait >= WaitInteval {
		maxRetry := int(maxWait / WaitInteval)
		opt.RetryStrategy = redislock.LimitRetry(redislock.LinearBackoff(WaitInteval), maxRetry)
		ll.Info("max wait for locker", zap.Duration("maxWait", maxWait))
	}
	ll.Info("request locker", zap.String("resource", resource), zap.Duration("timeout", timeout))
	lock, err := locker.Obtain(ctx, LockerPrefix+resource, timeout, opt)
	if err != nil {
		ll.Error("lock failed", zap.Error(err))
		if err == redislock.ErrNotObtained {
			ll.Warn("resource is locked.", zap.Error(err))
			return nil, errors.New(resource + " is locked.")
		}
		// panic("unexpected error, " + err.Error())
		return nil, err
	}
	ll.Debug("lock obtained")
	return func(ctx context.Context) error {
		err := lock.Release(context.Background())
		if err != nil {
			ll.Error("release locker failed. try to delete it", zap.Error(err))
			err = r.client.Del(context.Background(), resource).Err()
			if err != nil {
				ll.Error("delete locker failed", zap.Error(err))
				return err
			}
			ll.Info("deleted locker done.")
			return nil
		}
		ll.Debug("release locker done.")
		return nil
	}, nil
}

func (r *RedisLocker) LockWithtimeout(ctx context.Context, resource string, timeout time.Duration) (Release, error) {
	return r.WaitForLocker(ctx, resource, 0, timeout)
}

func (r *RedisLocker) Lock(ctx context.Context, resource string) (Release, error) {
	return r.WaitForLocker(ctx, resource, 0, 0)
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
