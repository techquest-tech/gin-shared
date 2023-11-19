//go:build redis

package cache

import (
	"context"
	"reflect"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

func NewRedisClient(logger *zap.Logger) *redis.Client {
	opts := &redis.Options{}

	subRedis := viper.Sub("redis")
	if subRedis != nil {
		subRedis.Unmarshal(opts)
	}
	client := redis.NewClient(opts)
	logger.Info("connected to redis", zap.String("redis", opts.Addr))
	return client
}

func NewCacheProvider[T any](t time.Duration) CacheProvider[T] {
	rr := &RedisProvider[T]{
		prefix:  randstr.String(3),
		timeout: t,
	}
	err := core.GetContainer().Invoke(func(client *redis.Client) {
		rrr := cache.New(&cache.Options{
			Redis:      client,
			LocalCache: cache.NewTinyLFU(1000, t),
		})
		rr.cache = rrr
	})
	if err != nil {
		zap.L().Error("new cache provider failed.", zap.Error(err))
	}
	return rr
}

type RedisProvider[T any] struct {
	prefix  string
	timeout time.Duration
	cache   *cache.Cache
}

// Get implements CacheProvider.
func (r *RedisProvider[T]) Get(key string) (T, bool) {
	k := r.prefix + key
	zap.L().Debug("try to read value from redis", zap.String("key", k))

	ctx := context.TODO()

	var value T
	if r.cache.Exists(ctx, k) {
		t := reflect.TypeOf(value)
		if t.Kind() != reflect.Ptr {
			if err := r.cache.Get(context.TODO(), k, &value); err != nil {
				return value, true
			}
		} else {
			var vv *T
			if err := r.cache.Get(context.TODO(), k, &vv); err != nil {
				r := *vv
				return r, true
			}
		}

	}

	return value, false
}

// Set implements CacheProvider.
func (r *RedisProvider[T]) Set(key string, value T) {
	err := r.cache.Set(&cache.Item{
		Ctx:   context.TODO(),
		Key:   r.prefix + key,
		Value: value,
		TTL:   r.timeout,
	})
	if err != nil {
		zap.L().Error("set cache failed.", zap.Error(err))
	}
}

func init() {
	core.Provide(NewRedisClient)
}
