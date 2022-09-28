package orm

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var entities = make([]interface{}, 0)

func AppendEntity(entity ...interface{}) {
	entities = append(entities, entity...)
}

func MigrateTableAndView(db *gorm.DB, logger *zap.Logger) {
	logger.Info("init all tables")
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
