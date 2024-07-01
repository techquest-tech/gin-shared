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
