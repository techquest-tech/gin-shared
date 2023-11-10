package schedule

import (
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DBCronJob struct {
	Name     string
	Schedule string
	Sql      []string
	logger   *zap.Logger
	db       *gorm.DB
	// cr       *cron.Cron
}

func (job *DBCronJob) FireJob() {
	// job.logger.Info("run db scheduled job")
	for _, item := range job.Sql {
		result := job.db.Exec(item)
		if result.Error != nil {
			job.logger.Error("run sql failed", zap.String("sql", item), zap.Error(result.Error))
			return
		}
	}
	job.logger.Info("all sql done")
}

func InitDBCronJob(logger *zap.Logger, db *gorm.DB) (core.Startup, error) {
	sub := viper.Sub("cronjob")
	if sub == nil {
		logger.Debug("not DB job is scheduled.")
		return nil, nil
	}

	for key := range sub.AllSettings() {
		item := &DBCronJob{
			logger:   logger.With(zap.String("job", key)),
			db:       db,
			Name:     key,
			Schedule: sub.GetString(key + ".schedule"),
			Sql:      sub.GetStringSlice(key + ".sql"),
		}
		if item.Schedule != "-" && len(item.Sql) > 0 {
			err := CreateSchedule(item.Name, item.Schedule, item.FireJob)
			if err != nil {
				item.logger.Error("start up schedule failed.", zap.Error(err))
				return nil, err
			}
		}
	}
	return nil, nil
}

func init() {
	core.ProvideStartup(InitDBCronJob)
}
