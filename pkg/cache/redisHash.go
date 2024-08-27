//go:build !ram

package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type RedisHashService struct {
	Client *redis.Client
	Logger *zap.Logger
}

func NewRedisHashService(client *redis.Client, logger *zap.Logger) Hash {
	return &RedisHashService{
		Client: client,
		Logger: logger,
	}
}

func (rs *RedisHashService) SetTTL(ctx context.Context, key string, ttl time.Duration) {
	rs.Client.Expire(ctx, key, ttl)
}

func (rs *RedisHashService) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	if len(fields) == 0 {
		rs.Logger.Warn("no fields to get redis hash")
		return nil, nil
	}
	resp, err := rs.Client.HMGet(ctx, key, fields...).Result()
	if err != nil {
		rs.Logger.Error("failed to get values from redis", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (rs *RedisHashService) SetValues(ctx context.Context, key string, values map[string]any) error {
	if len(values) == 0 {
		rs.Logger.Warn("no values to set redis hash")
		return nil
	}
	_, err := rs.Client.HSet(ctx, key, values).Result()
	if err != nil {
		rs.Logger.Error("failed to set values to redis", zap.Error(err))
		return err
	}
	rs.Logger.Info("set values done", zap.Int("len", len(values)))
	return nil
}

func init() {
	core.Provide(NewRedisHashService)
}
