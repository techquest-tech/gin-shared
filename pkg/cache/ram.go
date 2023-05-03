package cache

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var DefaultTimeout time.Duration = 30 * time.Minute

type Cache[T any] struct {
	cc cache.Cache
}

func NewWithTimeout[T any](dur time.Duration) *Cache[T] {
	r := &Cache[T]{
		cc: *cache.New(dur, 2*dur),
	}
	return r
}

func New[T any]() *Cache[T] {
	return (*Cache[T])(NewWithTimeout[T](DefaultTimeout))
}

func (ct *Cache[T]) Set(key string, value T) {
	ct.cc.SetDefault(key, value)
}

func (ct *Cache[T]) Get(key string) (T, bool) {
	if vv, found := ct.cc.Get(key); found {
		v := vv.(T)
		return v, true
	}
	var zero T
	return zero, false
}
