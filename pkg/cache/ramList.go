//go:build ram

package cache

import (
	"context"
	"errors"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type RamList[T any] struct {
	Logger  *zap.Logger
	Timeout time.Duration
	Ram     cache.Cache
	Prefix  string
}

func NewCachedList[T any](prefix string, dur time.Duration) CachedList[T] {
	return &RamList[T]{
		Logger:  zap.L(),
		Timeout: dur,
		Ram:     *cache.New(dur, 2*dur),
		Prefix:  prefix,
	}
}

func (rr *RamList[T]) Append(ctx context.Context, key string, raw ...T) error {
	k := rr.Prefix + key
	vv, found := rr.Ram.Get(k)
	if !found {
		r := lo.ToAnySlice(raw)
		rr.Ram.SetDefault(k, r)
	} else {
		s := vv.([]any)
		r := lo.ToAnySlice(raw)
		s = append(s, r...)
		rr.Ram.SetDefault(k, s)
	}
	return nil
}

func (rr *RamList[T]) GetAll(ctx context.Context, key string) ([]T, error) {
	if vv, found := rr.Ram.Get(rr.Prefix + key); found {
		v := vv.([]any)
		result := make([]T, len(v))
		for index, item := range v {
			result[index] = item.(T)
		}
		return result, nil
	}
	var vv []T
	return vv, errors.New(key + " not found in cache")
}

func (rr *RamList[T]) Del(ctx context.Context, key string) error {
	rr.Ram.Delete(rr.Prefix + key)
	return nil
}
