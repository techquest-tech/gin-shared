package schedule

import (
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DBCronJob struct {
	Name     string
	Schedule string
	Sql      []string
	Logger   *zap.Logger
	DB       *gorm.DB
}

func (job *DBCronJob) FireJob() {
	// job.logger.Info("run db scheduled job")
	for _, item := range job.Sql {
		result := job.DB.Exec(item)
		if result.Error != nil {
			job.Logger.Error("run sql failed", zap.String("sql", item), zap.Error(result.Error))
			return
		}
		job.Logger.Info("run sql job done", zap.String("sql", item), zap.Int64("rows", result.RowsAffected))
	}
	job.Logger.Info("all sql done")
}

func InitDBCronJob(logger *zap.Logger, db *gorm.DB) (core.Startup, error) {
	sub := viper.Sub("cronjob")
	if sub == nil {
		logger.Debug("not DB job is scheduled.")
		return nil, nil
	}

	for key := range sub.AllSettings() {
		item := &DBCronJob{
			Logger:   logger.With(zap.String("job", key)),
			DB:       db,
			Name:     key,
			Schedule: sub.GetString(key + ".schedule"),
			Sql:      sub.GetStringSlice(key + ".sql"),
		}
		replacedSql := make([]string, len(item.Sql))
		for index, sql := range item.Sql {
			replacedSql[index] = orm.ReplaceTablePrefix(sql)
		}
		item.Sql = replacedSql
		if item.Schedule != "-" && len(item.Sql) > 0 {
			err := CreateSchedule(item.Name, item.Schedule, item.FireJob)
			if err != nil {
				item.Logger.Error("start up schedule failed.", zap.Error(err))
				return nil, err
			}
		}
	}
	return nil, nil
}

func init() {
	core.ProvideStartup(InitDBCronJob)
}
