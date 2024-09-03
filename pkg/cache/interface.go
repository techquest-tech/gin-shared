package cache

import (
	"context"
	"fmt"
	"time"

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
