package gormcache

import (
	"context"
	"fmt"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormHashService struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewGormHashService(db *gorm.DB, logger *zap.Logger) cache.Hash {
	return &GormHashService{
		db:     db,
		logger: logger,
	}
}

func (gh *GormHashService) Existed(ctx context.Context, key string) (bool, error) {
	var count int64
	err := gh.db.Model(&HashEntry{}).Where("key = ? AND (expires_at IS NULL OR expires_at > ?)", key, time.Now()).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (gh *GormHashService) SetTTL(ctx context.Context, key string, ttl time.Duration) {
	expiresAt := time.Now().Add(ttl)
	err := gh.db.Model(&HashEntry{}).Where("key = ?", key).Update("expires_at", expiresAt).Error
	if err != nil {
		gh.logger.Error("gorm hash set ttl failed", zap.String("key", key), zap.Error(err))
		return
	}
	gh.logger.Info("set ttl done", zap.Duration("ttl", ttl), zap.String("key", key))
}

func (gh *GormHashService) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	if len(fields) == 0 {
		gh.logger.Warn("no fields to get gorm hash")
		return nil, nil
	}

	var entries []HashEntry
	err := gh.db.Where("key = ? AND field IN ? AND (expires_at IS NULL OR expires_at > ?)", key, fields, time.Now()).Find(&entries).Error
	if err != nil {
		gh.logger.Error("failed to get values from gorm", zap.Error(err))
		return nil, err
	}

	result := make([]any, len(fields))
	valueMap := make(map[string]string)
	for _, entry := range entries {
		valueMap[entry.Field] = entry.Value
	}

	for i, field := range fields {
		if val, ok := valueMap[field]; ok {
			result[i] = val
		} else {
			result[i] = nil
		}
	}

	return result, nil
}

func (gh *GormHashService) SetValues(ctx context.Context, key string, values map[string]any) error {
	if len(values) == 0 {
		gh.logger.Warn("no values to set gorm hash")
		return nil
	}

	entries := make([]HashEntry, 0, len(values))

	for field, value := range values {
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		entries = append(entries, HashEntry{
			Key:   key,
			Field: field,
			Value: valueStr,
		})
	}

	err := gh.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}, {Name: "field"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(entries).Error

	if err != nil {
		gh.logger.Error("failed to set values to gorm", zap.Error(err))
		return err
	}

	gh.logger.Info("set values done", zap.Int("len", len(values)), zap.String("key", key))
	return nil
}

func (gh *GormHashService) GetAll(ctx context.Context, key string) (map[string]string, error) {
	var entries []HashEntry
	err := gh.db.Where("key = ? AND (expires_at IS NULL OR expires_at > ?)", key, time.Now()).Find(&entries).Error
	if err != nil {
		gh.logger.Error("failed to get all values from gorm", zap.Error(err))
		return nil, err
	}

	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		result[entry.Field] = entry.Value
	}

	return result, nil
}

func init() {
	core.Provide(NewGormHashService)
}
