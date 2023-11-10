package locker

import (
	"context"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"summationsolutions.com/rfid/scm/pkg/common"
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

func (dl *DbLocker) Lock(ctx context.Context, resource string) (Release, error) {
	tx := dl.DB.Begin()

	_, ok := dl.cache[resource]
	if ok {
		n := time.Now()
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
		return nil
	}, nil
}

func InitDBLocker(p common.ServiceParam) *DbLocker {
	return &DbLocker{
		DB:     p.DB,
		Logger: p.Logger,
		cache:  map[string]bool{},
	}
}
