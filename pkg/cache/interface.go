package cache

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

type CacheProvider[T any] interface {
	Set(key string, value T)
	Get(key string) (T, bool)
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
		// cc: NewRedisCacheProvider[T](dur),
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
