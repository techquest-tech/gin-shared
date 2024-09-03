//go:build ram

package cache

import (
	"context"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func NewCacheProvider[T any](t time.Duration) CacheProvider[T] {
	rr := &CacheRam[T]{
		ram: *cache.New(t, 2*t),
	}
	return rr
}

type LocalRamHash struct {
	ram sync.Map
}

func (lhash *LocalRamHash) Existed(ctx context.Context, key string) (bool, error) {
	return true, nil
}
func (lhash *LocalRamHash) SetTTL(ctx context.Context, key string, ttl time.Duration) {
	zap.L().Info("local hash doesn't support TTL")
}

func (lhash *LocalRamHash) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	result := make([]any, len(fields))
	for index, item := range fields {
		if vv, found := lhash.ram.Load(key + item); found {
			result[index] = vv
		}
	}
	return result, nil
}
func (lhash *LocalRamHash) SetValues(ctx context.Context, key string, values map[string]any) error {
	for k, v := range values {
		lhash.ram.Store(key+k, v)
	}
	return nil
}

func NewLocalRamHash() Hash {
	return &LocalRamHash{}
}
func init() {
	core.Provide(NewLocalRamHash)
}
