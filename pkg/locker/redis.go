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
	locker := redislock.New(r.client)
	if timeout == 0 {
		timeout = MaxLockerDuration
	}
	opt := &redislock.Options{}

	if maxWait >= WaitInteval {
		maxRetry := int(maxWait / WaitInteval)
		opt.RetryStrategy = redislock.LimitRetry(redislock.LinearBackoff(WaitInteval), maxRetry)
		r.Logger.Info("max wait for locker", zap.Duration("maxWait", maxWait))
	}

	lock, err := locker.Obtain(ctx, LockerPrefix+resource, timeout, opt)
	if err != nil {
		r.Logger.Error("lock failed", zap.Error(err))
		if err == redislock.ErrNotObtained {
			r.Logger.Warn("resource is locked.", zap.Error(err), zap.String("resource", resource))
			return nil, errors.New(resource + " is locked.")
		}
		// panic("unexpected error, " + err.Error())
		return nil, err
	}
	r.Logger.Debug("lock obtained", zap.String("resource", resource))
	return func(ctx context.Context) error {
		err := lock.Release(context.Background())
		if err != nil {
			r.Logger.Error("release locker failed. try to delete it", zap.Error(err), zap.String("resource", resource))
			err = r.client.Del(context.Background(), resource).Err()
			if err != nil {
				r.Logger.Error("delete locker failed", zap.Error(err), zap.String("resource", resource))
				return err
			}
			r.Logger.Info("deleted locker done.")
			return nil
		}
		r.Logger.Debug("release locker done.", zap.String("resource", resource))
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
