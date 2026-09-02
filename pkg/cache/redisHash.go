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
	Client    *redis.Client
	Logger    *zap.Logger
	gate      *healthGate
	writeGate *healthGate
}

func NewRedisHashService(client *redis.Client, logger *zap.Logger) Hash {
	return &RedisHashService{
		Client:    client,
		Logger:    logger,
		gate:      redisGate,
		writeGate: redisWriteGate,
	}
}

func (rs *RedisHashService) Existed(ctx context.Context, key string) (bool, error) {
	if !rs.gate.allow() {
		return false, ErrCacheUnavailable
	}
	rr := rs.Client.Exists(ctx, key)
	if rr.Err() != nil {
		rs.gate.fail()
		return false, rr.Err()
	}
	return rr.Val() > 0, nil
}

func (rs *RedisHashService) SetTTL(ctx context.Context, key string, ttl time.Duration) {
	if !rs.gate.allow() || !rs.writeGate.allow() {
		return
	}
	r, err := rs.Client.Expire(ctx, key, ttl).Result()
	if err != nil {
		rs.writeGate.fail()
		rs.Logger.Error("redis hash set ttl failed", zap.String("key", key), zap.Error(err))
		return
	}
	rs.Logger.Info("set ttl result", zap.Bool("result", r), zap.Duration("ttl", ttl))
}

func (rs *RedisHashService) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	if len(fields) == 0 {
		rs.Logger.Warn("no fields to get redis hash")
		return nil, nil
	}
	if !rs.gate.allow() {
		return nil, ErrCacheUnavailable
	}
	resp, err := rs.Client.HMGet(ctx, key, fields...).Result()
	if err != nil {
		rs.gate.fail()
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
	if !rs.gate.allow() || !rs.writeGate.allow() {
		return ErrCacheUnavailable
	}
	_, err := rs.Client.HSet(ctx, key, values).Result()
	if err != nil {
		// 写失败只熔断「写」，不熔断「读」：内存满(OOM)时写会失败但读仍可用。
		rs.writeGate.fail()
		rs.Logger.Error("failed to set values to redis", zap.Error(err))
		return err
	}
	rs.Logger.Info("set values done", zap.Int("len", len(values)))
	return nil
}

func (rs *RedisHashService) GetAll(ctx context.Context, key string) (map[string]string, error) {
	if !rs.gate.allow() {
		return nil, ErrCacheUnavailable
	}
	resp, err := rs.Client.HGetAll(ctx, key).Result()
	if err != nil {
		rs.gate.fail()
		rs.Logger.Error("failed to get all values from redis", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func init() {
	core.Provide(NewRedisHashService)
}
