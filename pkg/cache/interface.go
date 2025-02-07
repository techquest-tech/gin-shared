package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	"go.uber.org/zap"
)

type WithKey interface {
	Key() string
}

type CachedList[T any] interface {
	Append(ctx context.Context, key string, raw ...T) error
	GetAll(ctx context.Context, key string) ([]T, error)
	Del(ctx context.Context, key string) error
}

type Hash interface {
	Existed(ctx context.Context, key string) (bool, error)
	SetTTL(ctx context.Context, key string, ttl time.Duration)
	GetValues(ctx context.Context, key string, fields ...string) ([]any, error)
	SetValues(ctx context.Context, key string, values map[string]any) error
}

type CacheProvider[T any] interface {
	Set(key string, value T)
	Get(key string) (T, bool)
	Del(key string) error
	Keys() []string
}

// type CacheFactory[T any] interface {
// 	New(t time.Duration) CacheProvider[T]
// }

var DefaultTimeout time.Duration = 30 * time.Minute

type Cache[T any] struct {
	cc CacheProvider[T]
}

func NewWithTimeout[T any](dur time.Duration) *Cache[T] {
	r := &Cache[T]{
		cc: NewCacheProvider[T](dur),
	}
	zap.L().Info("cache provider ready.", zap.String("provider", fmt.Sprintf("%T", r.cc)))
	return r
}

func New[T any]() *Cache[T] {
	return NewWithTimeout[T](DefaultTimeout)
}

func (ct *Cache[T]) Set(key string, value T) {
	ct.cc.Set(key, value)
}

func (ct *Cache[T]) Get(key string) (T, bool) {
	if vv, found := ct.cc.Get(key); found {
		return vv, true
	}
	var zero T
	return zero, false
}
func (ct *Cache[T]) Keys() []string {
	return ct.cc.Keys()
}
func (ct *Cache[T]) Del(key string) error {
	return ct.cc.Del(key)
}

type CachedResult[T any, R any] struct {
	Cache *Cache[T]
}

func NewCachedResult[T any, R any](timeout time.Duration) *CachedResult[T, R] {
	return &CachedResult[T, R]{
		Cache: NewWithTimeout[T](timeout),
	}
}

func hashKey(req any) (string, error) {
	switch v := req.(type) {
	case WithKey:
		return v.Key(), nil
	case int:
		return strconv.FormatInt(int64(v), 36), nil
	case int32:
		return strconv.FormatInt(int64(v), 36), nil
	case int64:
		return strconv.FormatInt(v, 36), nil
	case uint:
		return strconv.FormatUint(uint64(v), 36), nil
	case uint32:
		return strconv.FormatUint(uint64(v), 36), nil
	case uint64:
		return strconv.FormatUint(v, 36), nil
	default:
		hashed, err := hashstructure.Hash(v, hashstructure.FormatV2, nil)
		if err != nil {
			zap.L().Error("hash key failed", zap.Error(err))
		}
		return strconv.FormatUint(hashed, 36), nil
	}
}

func (cc *CachedResult[T, R]) CacheFun1(ctx context.Context, req R, fn func(ctx context.Context, req R) (T, error)) (T, error) {
	if cc.Cache == nil {
		return fn(ctx, req)
	}
	key, err := hashKey(req)
	if err != nil {
		zap.L().Error("hash key failed, call fn directly.")
		return fn(ctx, req)
	}
	if vv, found := cc.Cache.Get(key); found {
		zap.L().Info("cache hit.", zap.String("key", key))
		return vv, nil
	}
	r, err := fn(ctx, req)
	if err != nil {
		zap.L().Error("call fn failed.", zap.Error(err))
		return r, err
	}

	cc.Cache.Set(key, r)

	return r, nil
}
