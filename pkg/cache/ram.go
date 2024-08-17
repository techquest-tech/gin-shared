package cache

import (
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
)

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
func (cc *CacheRam[T]) Keys() []string {
	all := cc.ram.Items()
	return lo.Keys(all)
}
func (cc *CacheRam[T]) Del(key string) error {
	cc.ram.Delete(key)
	return nil
}

func NewRAMCacheProvider[T any](t time.Duration) *CacheRam[T] {
	rr := &CacheRam[T]{
		ram: *cache.New(t, 2*t),
	}
	return rr
}
