package cache

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var DefaultTimeout time.Duration = 30 * time.Minute

type Cache[T any] struct {
	cc cache.Cache
}

func New[T any]() *Cache[T] {
	r := &Cache[T]{
		cc: *cache.New(DefaultTimeout, 2*DefaultTimeout),
	}
	return r
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
