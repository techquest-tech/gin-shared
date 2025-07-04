package orm

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func init() {
	ginshared.GetContainer().Provide(InitDefaultDB)
}

const (
	KeyInitDB      = ginshared.KeyInitDB
	KeyTablePrefix = "database.tablePrefix"
)

type OrmDialector func(dsn string) gorm.Dialector

var (
	DialectorMap = make(map[string]OrmDialector)
	Connections  = make(map[string]*gorm.DB)
)

func InitDefaultDB(logger *zap.Logger) *gorm.DB {
	return InitDBWithPrefix("database", "")
}

func SessionWithConfig(slowThreshold time.Duration, ignoredNotFound bool) *gorm.Session {
	return &gorm.Session{
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             slowThreshold,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: ignoredNotFound,
				Colorful:                  true,
			}),
	}
}

func InitDBWithPrefix(sub string, prefix string) *gorm.DB {
	logger := zap.L()
	dbSettings := viper.Sub(sub)
	if dbSettings == nil {
		return nil
	}

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

	tablePrefix := dbSettings.GetString("tablePrefix")
	if prefix != "" {
		tablePrefix = prefix + "_" + tablePrefix
	}

	if tablePrefix != "" {
		cfg.NamingStrategy = schema.NamingStrategy{
			TablePrefix: tablePrefix,
		}
		logger.Info("user table prefix", zap.String("tableprefix", tablePrefix))
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
}

func InitDB(sub string, logger *zap.Logger) *gorm.DB {
	return InitDBWithPrefix(sub, "")
}
