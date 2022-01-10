package orm

import (
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
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
	job.logger.Info("run update warehouse code job")
	result := job.db.Exec(job.Sql)

	if result.Error != nil {
		job.logger.Error("job failed", zap.Error(result.Error))
	}
	next := job.cr.Entries()[0].Next
	job.logger.Info("Job done.", zap.Int64("updated", result.RowsAffected), zap.Time("next", next))
}

type CronZaplog struct {
	logger *zap.Logger
}

func (c *CronZaplog) Info(msg string, keysAndValues ...interface{}) {
	args := []interface{}{msg}
	args = append(args, keysAndValues...)
	c.logger.Sugar().Info(args...)
}
func (c *CronZaplog) Error(err error, msg string, keysAndValues ...interface{}) {
	args := []interface{}{err.Error() + "," + msg}
	args = append(args, keysAndValues...)
	c.logger.Sugar().Error(args...)
}

func CreateSchedule(jobname, schedule string, cmd func(), logger *zap.Logger) (*cron.Cron, error) {
	l := &CronZaplog{
		logger: logger,
	}
	cr := cron.New(cron.WithChain(cron.SkipIfStillRunning(l)))
	_, err := cr.AddFunc(schedule, cmd)
	if err != nil {
		return nil, err
	}
	cr.Start()

	entries := cr.Entries()
	nextRuntime := entries[0].Next

	logger.Info("cron job scheduled", zap.String("job", jobname), zap.String("schedule", schedule), zap.Time("next", nextRuntime))

	return cr, nil
}

func init() {
	ginshared.GetContainer().Provide(func(logger *zap.Logger, db *gorm.DB) (ginshared.DiController, error) {
		sub := viper.Sub("cronjob")
		if sub != nil {
			jobs := make([]*DBCronJob, 0)
			err := sub.Unmarshal(&jobs)
			if err != nil {
				logger.Error("cronjob settings error,", zap.Error(err))
				return nil, err
			}

			for _, item := range jobs {
				item.db = db
				item.logger = logger.With(zap.String("job", item.Name))
				if item.Schedule != "-" {
					cr, err := CreateSchedule(item.Name, item.Schedule, item.FireJob, item.logger)
					if err != nil {
						item.logger.Error("start up schedule failed.", zap.Error(err))
						return nil, err
					}
					item.cr = cr
					item.logger.Info("job scheduled")
				}
			}
			return jobs, nil
		} else {
			logger.Info("not db job scheduled.")
		}
		return nil, nil
	})
}
