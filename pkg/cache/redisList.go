//go:build !ram

package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type CachedListRedis[T any] struct {
	Logger  *zap.Logger
	Client  *redis.Client
	Prefix  string
	Timeout time.Duration
}

func NewCachedList[T any](prefix string, dur time.Duration) CachedList[T] {
	var result CachedList[T]
	err := core.GetContainer().Invoke(func(client *redis.Client) {
		result = &CachedListRedis[T]{
			Logger:  zap.L(),
			Prefix:  prefix,
			Timeout: dur,
			Client:  client,
		}
	})
	if err != nil {
		panic("init cachedList impl failed." + err.Error())
	}
	return result
}

func (ll *CachedListRedis[T]) Append(ctx context.Context, key string, raw ...T) error {
	rKey := ll.Prefix + key
	reqs := make([]any, 0)
	for _, item := range raw {
		data, err := json.Marshal(&item)
		if err != nil {
			ll.Logger.Error("marshal data failed.", zap.Error(err))
			return err
		}
		reqs = append(reqs, string(data))
	}
	resp := ll.Client.LPush(ctx, rKey, reqs...)
	if resp.Err() != nil {
		ll.Logger.Error("push data to redis failed.", zap.Error(resp.Err()))
		return resp.Err()
	}
	ll.Logger.Debug("push done.", zap.Any("resp", resp))

	if ll.Timeout > 0 {
		ll.Client.Expire(ctx, rKey, ll.Timeout)
	}

	return nil
}

func hasKey(raw any) string {
	if kk, ok := raw.(WithKey); ok {
		return kk.Key()
	}
	return ""
}

func (ll *CachedListRedis[T]) GetAll(ctx context.Context, key string) ([]T, error) {
	resp := ll.Client.LRange(ctx, ll.Prefix+key, 0, -1)
	if resp.Err() != nil {
		ll.Logger.Error("read all cached items failed.", zap.Error(resp.Err()))
		return nil, resp.Err()
	}
	strs := resp.Val()
	keys := map[string]bool{}
	result := make([]T, 0)
	for _, item := range strs {
		var t T
		err := json.Unmarshal([]byte(item), &t)
		if err != nil {
			ll.Logger.Error("not expected json format, unmarshal failed.", zap.Error(err), zap.String("raw", item))
			return nil, err
		}

		k := hasKey(t)
		if k != "" {
			if _, ok := keys[k]; ok {
				ll.Logger.Info("item is duplicated, ignored.", zap.Any("item", t))
				continue
			}
			keys[k] = true
		}
		result = append(result, t)
	}
	return result, nil
}

func (ll *CachedListRedis[T]) Del(ctx context.Context, key string) error {
	resp := ll.Client.Del(ctx, ll.Prefix+key)
	if resp.Err() != nil {
		return resp.Err()
	}
	ll.Logger.Info("cache removed.", zap.String("key", key))
	return nil
}
