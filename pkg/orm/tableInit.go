package orm

import (
	"fmt"
	"sync"

	"github.com/asaskevich/EventBus"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var entities = make([]interface{}, 0)

func AppendEntity(entity ...interface{}) {
	entities = append(entities, entity...)
}

var ormOnce sync.Once

func ApplyMigrate() error {
	fn := func(db *gorm.DB, logger *zap.Logger, bus core.OptionalParam[EventBus.Bus]) {
		ormOnce.Do(func() {
			if viper.GetBool(KeyInitDB) {
				MigrateTableAndView(db, logger, bus.P)
			}
		})
	}
	core.InvokeAsyncOnServiceStarted(fn)
	return nil
}

func MigrateTableAndView(db *gorm.DB, logger *zap.Logger, bus EventBus.Bus) {
	logger.Info("init all tables")

	for _, item := range entities {
		name := fmt.Sprintf("%T", item)
		zap.L().Info("applied entity", zap.String("entity", name))
	}
	err := db.AutoMigrate(entities...)
	if err != nil {
		logger.Error("init tables failed", zap.Error(err))
	} else {
		if bus != nil {
			bus.Publish("sys.db.inited")
		}
		logger.Info("init tables done")
	}

	err = InitMysqlViews(db, logger)
	if err != nil {
		logger.Error("init views failed", zap.Error(err))
	} else {
		logger.Info("init views done.")
	}
}

// func init() {
// 	AppendEntity(&schedule.JobSchedule{})
// }
