package orm

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var entities = make([]interface{}, 0)

func AppendEntity(entity ...interface{}) {
	entities = append(entities, entity...)
}

func ApplyMigrate() error {
	return core.Container.Invoke(core.Container.Invoke(func(db *gorm.DB, logger *zap.Logger) {
		if viper.GetBool(KeyInitDB) {
			MigrateTableAndView(db, logger)
		}
		viper.Set(KeyInitDB, false) //just make sure the ApplyMigrate run once only
	}))
}

func MigrateTableAndView(db *gorm.DB, logger *zap.Logger) {
	logger.Info("init all tables")

	for _, item := range entities {
		name := fmt.Sprintf("%T", item)
		zap.L().Info("applied entity", zap.String("entity", name))
	}
	err := db.AutoMigrate(entities...)
	if err != nil {
		logger.Error("init tables failed", zap.Error(err))
	} else {
		logger.Info("init tables done")
	}

	err = InitMysqlViews(db, logger)
	if err != nil {
		logger.Error("init views failed", zap.Error(err))
	} else {
		logger.Info("init views done.")
	}

}
