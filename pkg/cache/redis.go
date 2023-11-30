//go:build redis

package cache

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
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
		// prefix:  randstr.Hex(4) + "-",
		timeout: t,
	}
	tname := fmt.Sprintf("%T", rr)

	from := strings.LastIndexByte(tname, '.')
	from2 := strings.LastIndexByte(tname, '[')

	if from < from2 {
		from = from2
	}

	if from > 0 {
		from = from + 1
	}
	to := strings.LastIndexByte(tname, ']')

	rr.prefix = tname[from:to] + "-"

	err := core.GetContainer().Invoke(func(client *redis.Client) {
		rrr := cache.New(&cache.Options{
			Redis:      client,
			LocalCache: cache.NewTinyLFU(1000, t),
		})
		rr.cache = rrr
	})
	if err != nil {
		zap.L().Error("new cache provider failed.", zap.Error(err))
		// panic(err)
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
	var value T
	if r.cache == nil {
		zap.L().Warn("redis cache is not functional now, ")
		return value, false
	}
	k := r.prefix + key
	zap.L().Debug("try to read value from redis", zap.String("key", k))

	ctx := context.TODO()
	if r.cache.Exists(ctx, k) {
		t := reflect.TypeOf(value)
		if t.Kind() != reflect.Ptr {
			err := r.cache.Get(context.TODO(), k, &value)
			if err != nil {
				zap.L().Error("read redis cache failed.", zap.Error(err))
				return value, false
			}
			zap.L().Debug("read value from redis done", zap.String("key", k), zap.Any("value", value))
			return value, true
		} else {
			var vv *T
			err := r.cache.Get(context.TODO(), k, &vv)
			if err != nil {
				zap.L().Error("read redis cache failed.", zap.Error(err))
				return value, false
			}
			r := *vv
			zap.L().Debug("read value from redis done", zap.String("key", k), zap.Any("value", r))
			return r, true
		}
	}
	zap.L().Debug("no value from redis", zap.String("key", k))
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
	zap.L().Info("set cache done", zap.String("key", r.prefix+key))
}

func init() {
	core.Provide(NewRedisClient)
}
