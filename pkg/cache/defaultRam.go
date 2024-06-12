//go:build ram

package cache

import (
	"time"

	"github.com/patrickmn/go-cache"
)

func NewCacheProvider[T any](t time.Duration) CacheProvider[T] {
	rr := &CacheRam[T]{
		ram: *cache.New(t, 2*t),
	}
	return rr
}

type CacheRam[T any] struct {
	ram cache.Cache
}

func (cc *CacheRam[T]) Set(key string, value T) {
	cc.ram.SetDefault(key, value)
}

func (cc *CacheRam[T]) Get(key string) (T, bool) {
	if vv, found := cc.ram.Get(key); found {
		v := vv.(T)
		return v, true
	}
	var vv T
	return vv, false
}
