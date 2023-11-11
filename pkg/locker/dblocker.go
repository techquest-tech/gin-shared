//go:build locker_db

package locker

import (
	"context"
	"sync"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ResourceLocker struct {
	Name      string `gorm:"size:128;primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func init() {
	orm.AppendEntity(&ResourceLocker{})
}

type DbLocker struct {
	DB     *gorm.DB
	Logger *zap.Logger
	cache  map[string]bool
}

var glocker sync.Mutex

func (dl *DbLocker) init() error {
	resources := make([]string, 0)
	err := dl.DB.Model(&ResourceLocker{}).Select("name").Find(&resources).Error
	if err != nil {
		return err
	}

	for _, item := range resources {
		dl.cache[item] = true
	}
	return nil
}

func (dl *DbLocker) LockWithtimeout(ctx context.Context, resource string, timeout time.Duration) (Release, error) {
	tx := dl.DB.Begin()

	var cancel context.CancelFunc

	_, ok := dl.cache[resource]
	if ok {
		n := time.Now()
		if timeout > 0 {
			nctx, c := context.WithTimeout(ctx, timeout)
			cancel = c
			tx = tx.WithContext(nctx)
		}
		err := tx.Model(&ResourceLocker{}).Where("name = ?", resource).Update("updated_at", n).Error
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	} else {
		glocker.Lock()
		defer glocker.Unlock()

		ll := &ResourceLocker{
			Name:      resource,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := tx.Save(ll).Error
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		dl.cache[resource] = true
	}
	dl.Logger.Debug("get locker done", zap.String("resource", resource))

	return func(ctx context.Context) error {
		tx.Commit()
		dl.Logger.Debug("release locker done", zap.String("resource", resource))
		if cancel != nil {
			cancel()
		}
		return nil
	}, nil
}

func (dl *DbLocker) Lock(ctx context.Context, resource string) (Release, error) {
	return dl.LockWithtimeout(ctx, resource, 0)
}

func InitDBLocker(db *gorm.DB, logger *zap.Logger) Locker {
	result := &DbLocker{
		DB:     db,
		Logger: logger,
		cache:  map[string]bool{},
	}
	result.init()
	return result
}

func init() {
	core.Provide(InitDBLocker)
}
