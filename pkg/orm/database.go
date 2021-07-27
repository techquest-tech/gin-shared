package orm

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func InitDB(logger *zap.Logger) *gorm.DB {
	dbSettings := viper.Sub("database")

	dbSettings.SetDefault("type", "mysql")

	dbType := dbSettings.GetString("type")

	uri := dbSettings.GetString("connection")
	maxLifetime := dbSettings.GetDuration("maxLifetime")
	max := dbSettings.GetInt("max")
	idel := dbSettings.GetInt("idel")

	db, err := gorm.Open(dbType, uri)

	if err != nil {
		panic(fmt.Sprintf("connect to db failed. err: %+v", err))
	}

	// See "Important settings" section.
	pool := db.DB()
	pool.SetConnMaxIdleTime(maxLifetime)
	pool.SetMaxOpenConns(max)
	pool.SetMaxIdleConns(idel)

	err = pool.Ping()
	if err != nil {
		panic(fmt.Errorf("connect to %s failed, %v", dbType, err))
	}

	// pool = db
	logger.Info("connected to " + dbType)

	return db

	// go bus.Publish(EventDBReady)
}
