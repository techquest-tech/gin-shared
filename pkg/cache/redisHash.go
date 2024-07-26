//go:build !ram

package cache

import (
	"context"

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

func (rs *RedisHashService) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	resp, err := rs.Client.HMGet(ctx, key, fields...).Result()
	if err != nil {
		rs.Logger.Error("failed to get values from redis", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (rs *RedisHashService) SetValues(ctx context.Context, key string, values map[string]string) error {
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
