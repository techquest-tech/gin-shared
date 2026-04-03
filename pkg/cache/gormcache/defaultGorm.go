//go:build gorm_cache

package gormcache

import (
	"context"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

// NewCacheProvider creates a new GORM cache provider
func NewCacheProvider[T any](t time.Duration) *GormCacheProvider[T] {
	return NewGormCacheProvider[T](t)
}

// GormHash implements Hash interface for GORM

type GormHash struct {
	// Implementation details
}

func (gh *GormHash) Existed(ctx context.Context, key string) (bool, error) {
	// Implementation for GORM hash
	return true, nil
}

func (gh *GormHash) SetTTL(ctx context.Context, key string, ttl time.Duration) {
	// Implementation for GORM hash TTL
	zap.L().Info("GORM hash doesn't support TTL")
}

func (gh *GormHash) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	// Implementation for GORM hash get values
	// This would use GORM to query hash-like data
	return []any{}, nil
}

func (gh *GormHash) SetValues(ctx context.Context, key string, values map[string]any) error {
	// Implementation for GORM hash set values
	// This would use GORM to store hash-like data
	return nil
}

func (gh *GormHash) GetAll(ctx context.Context, key string) (map[string]string, error) {
	// Implementation for GORM hash get all
	// This would use GORM to query all hash fields
	return map[string]string{}, nil
}

// NewGormHash creates a new GORM hash implementation
func NewGormHash() interface{} {
	return &GormHash{}
}

func init() {
	// Register GORM cache providers with the core container
	core.Provide(NewGormHash)
}
