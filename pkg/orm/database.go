package orm

import (
	"fmt"

	"github.com/spf13/viper"
	ginshared "github.com/techquest-tech/gin-shared/pkg/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func init() {
	ginshared.GetContainer().Provide(InitDB)
}

type OrmDialector func(dsn string) gorm.Dialector

var DialectorMap = make(map[string]OrmDialector)

func InitDB(logger *zap.Logger) *gorm.DB {
	dbSettings := viper.Sub("database")

	dbSettings.SetDefault("type", "mysql")

	dbType := dbSettings.GetString("type")

	uri := dbSettings.GetString("connection")
	maxLifetime := dbSettings.GetDuration("maxLifetime")
	max := dbSettings.GetInt("max")
	idel := dbSettings.GetInt("idel")

	f, ok := DialectorMap[dbType]

	if !ok {
		panic(fmt.Errorf("driver %s is missed", dbType))
	}

	cfg := &gorm.Config{
		PrepareStmt: true,
	}
	cfgorm := dbSettings.Sub("gorm")
	if cfgorm != nil {
		cfgorm.Unmarshal(cfg)
	}

	db, err := gorm.Open(f(uri), cfg)

	if err != nil {
		panic(fmt.Sprintf("connect to db failed. err: %+v", err))
	}

	// See "Important settings" section.
	pool, _ := db.DB()
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
