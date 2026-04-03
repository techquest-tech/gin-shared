package gormcache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type GormListService[T any] struct {
	db      *gorm.DB
	logger  *zap.Logger
	prefix  string
	timeout time.Duration
}

func NewGormListService[T any](prefix string, dur time.Duration) cache.CachedList[T] {
	var result cache.CachedList[T]
	err := core.GetContainer().Invoke(func(db *gorm.DB) {
		result = &GormListService[T]{
			db:      db,
			logger:  zap.L(),
			prefix:  prefix,
			timeout: dur,
		}
	})
	if err != nil {
		panic("init gorm cachedList impl failed." + err.Error())
	}
	return result
}

func (gl *GormListService[T]) Append(ctx context.Context, key string, raw ...T) error {
	rKey := gl.prefix + key

	entries := make([]ListEntry, 0, len(raw))
	now := time.Now()

	var maxSortOrder int
	err := gl.db.Model(&ListEntry{}).Where("key = ?", rKey).Select("COALESCE(MAX(sort_order), -1)").Scan(&maxSortOrder).Error
	if err != nil {
		gl.logger.Error("get max sort order failed", zap.Error(err))
		maxSortOrder = -1
	}

	for i, item := range raw {
		data, err := json.Marshal(&item)
		if err != nil {
			gl.logger.Error("marshal data failed", zap.Error(err))
			return err
		}

		entry := ListEntry{
			Key:       rKey,
			Value:     string(data),
			SortOrder: maxSortOrder + 1 + i,
		}

		if gl.timeout > 0 {
			expiresAt := now.Add(gl.timeout)
			entry.ExpiresAt = &expiresAt
		}

		entries = append(entries, entry)
	}

	err = gl.db.Create(&entries).Error
	if err != nil {
		gl.logger.Error("append list entries failed", zap.Error(err))
		return err
	}

	gl.logger.Debug("append list entries done", zap.Int("count", len(entries)), zap.String("key", rKey))
	return nil
}

func (gl *GormListService[T]) GetAll(ctx context.Context, key string) ([]T, error) {
	rKey := gl.prefix + key

	var entries []ListEntry
	err := gl.db.Where("key = ? AND (expires_at IS NULL OR expires_at > ?)", rKey, time.Now()).
		Order("sort_order ASC").
		Find(&entries).Error

	if err != nil {
		gl.logger.Error("read all list entries failed", zap.Error(err))
		return nil, err
	}

	keys := map[string]bool{}
	result := make([]T, 0, len(entries))

	for _, entry := range entries {
		var item T
		err := json.Unmarshal([]byte(entry.Value), &item)
		if err != nil {
			gl.logger.Error("unmarshal list entry failed", zap.Error(err), zap.String("value", entry.Value))
			return nil, err
		}

		k := hasKey(item)
		if k != "" {
			if _, ok := keys[k]; ok {
				gl.logger.Info("item is duplicated, ignored", zap.Any("item", item))
				continue
			}
			keys[k] = true
		}

		result = append(result, item)
	}

	return result, nil
}

func (gl *GormListService[T]) Del(ctx context.Context, key string) error {
	rKey := gl.prefix + key

	err := gl.db.Where("key = ?", rKey).Delete(&ListEntry{}).Error
	if err != nil {
		gl.logger.Error("delete list entries failed", zap.Error(err))
		return err
	}

	gl.logger.Info("list cache removed", zap.String("key", key))
	return nil
}

func hasKey(raw any) string {
	if kk, ok := raw.(cache.WithKey); ok {
		return kk.Key()
	}
	return ""
}
