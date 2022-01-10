package orm

import (
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/schedule"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DBCronJob struct {
	Name     string
	Schedule string
	Sql      string
	logger   *zap.Logger
	db       *gorm.DB
	cr       *cron.Cron
}

func (job *DBCronJob) FireJob() {
	job.logger.Info("run db scheduled job")
	result := job.db.Exec(job.Sql)

	if result.Error != nil {
		job.logger.Error("job failed", zap.Error(result.Error))
	}
	next := job.cr.Entries()[0].Next
	job.logger.Info("Job done.", zap.Int64("updated", result.RowsAffected), zap.Time("next", next))
}

func init() {
	ginshared.GetContainer().Provide(func(logger *zap.Logger, db *gorm.DB) (ginshared.DiController, error) {
		sub := viper.Sub("cronjob")
		if sub == nil {
			logger.Info("not DB job is scheduled.")
			return nil, nil
		}

		for key, _ := range sub.AllSettings() {
			item := &DBCronJob{
				logger:   logger.With(zap.String("job", key)),
				db:       db,
				Name:     key,
				Schedule: sub.GetString(key + ".Schedule"),
				Sql:      sub.GetString(key + ".Sql"),
			}
			if item.Schedule != "-" {
				cr, err := schedule.CreateSchedule(item.Name, item.Schedule, item.FireJob, item.logger)
				if err != nil {
					item.logger.Error("start up schedule failed.", zap.Error(err))
					return nil, err
				}
				item.cr = cr
				// item.logger.Info("job scheduled")
			}
		}
		return nil, nil
	}, ginshared.ControllerOptions)
}
