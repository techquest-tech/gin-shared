//go:build gorm_cache

package gormcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormCacheProvider[T any] struct {
	db      *gorm.DB
	prefix  string
	timeout time.Duration
}

func NewGormCacheProvider[T any](t time.Duration) *GormCacheProvider[T] {
	rr := &GormCacheProvider[T]{
		timeout: t,
	}

	tname := fmt.Sprintf("%T", rr)
	from := strings.LastIndexByte(tname, '.')
	from2 := strings.LastIndexByte(tname, '[')

	if from < from2 {
		from = from2
	}

	if from > 0 {
		from = from + 1
	}

	to := strings.LastIndexByte(tname, ']')
	prefix := tname[from:to]

	rr.prefix = prefix + "-"

	err := core.GetContainer().Invoke(func(db *gorm.DB) {
		rr.db = db
	})

	if err != nil {
		zap.L().Error("new gorm cache provider failed.", zap.Error(err))
		panic(err)
	}

	return rr
}

func (g *GormCacheProvider[T]) Set(key string, value T) {
	data, err := json.Marshal(value)
	if err != nil {
		zap.L().Error("marshal cache value failed", zap.Error(err))
		return
	}

	now := time.Now()
	expiresAt := now.Add(g.timeout)

	entry := &CacheEntry{
		Key:       g.prefix + key,
		Value:     string(data),
		ExpiresAt: &expiresAt,
	}

	err = g.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{"value": entry.Value, "expires_at": entry.ExpiresAt, "updated_at": now}),
	}).Create(entry).Error

	if err != nil {
		zap.L().Error("set cache entry failed", zap.Error(err), zap.String("key", entry.Key))
		return
	}

	zap.L().Debug("set cache entry done", zap.String("key", entry.Key))
}

func (g *GormCacheProvider[T]) Get(key string) (T, bool) {
	var zero T

	entry := &CacheEntry{}
	err := g.db.Where("key = ? AND (expires_at IS NULL OR expires_at > ?)", g.prefix+key, time.Now()).First(entry).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			zap.L().Debug("cache entry not found", zap.String("key", g.prefix+key))
			return zero, false
		}
		zap.L().Error("get cache entry failed", zap.Error(err), zap.String("key", g.prefix+key))
		return zero, false
	}

	var value T
	err = json.Unmarshal([]byte(entry.Value), &value)
	if err != nil {
		zap.L().Error("unmarshal cache value failed", zap.Error(err))
		return zero, false
	}

	zap.L().Debug("cache hit", zap.String("key", g.prefix+key))
	return value, true
}

func (g *GormCacheProvider[T]) Del(key string) error {
	err := g.db.Where("key = ?", g.prefix+key).Delete(&CacheEntry{}).Error
	if err != nil {
		zap.L().Error("delete cache entry failed", zap.Error(err), zap.String("key", g.prefix+key))
		return err
	}
	zap.L().Debug("delete cache entry done", zap.String("key", g.prefix+key))
	return nil
}

func (g *GormCacheProvider[T]) Keys() []string {
	var entries []CacheEntry
	err := g.db.Select("key").Where("key LIKE ?", g.prefix+"%").Find(&entries).Error
	if err != nil {
		zap.L().Error("get cache keys failed", zap.Error(err))
		return []string{}
	}

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		k := strings.TrimPrefix(entry.Key, g.prefix)
		if k != "" {
			keys = append(keys, k)
		}
	}

	return keys
}
