package schedule

import (
	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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

type JobParams struct {
	dig.In
	DB     *gorm.DB `optional:"true"`
	Logger *zap.Logger
	Bus    EventBus.Bus `optional:"true"`
}

func CreateSchedule(jobname, schedule string, cmd func(), aa ...any) error {
	err := core.GetContainer().Invoke(func(p JobParams) error {
		l := &CronZaplog{
			logger: p.Logger,
		}
		opts := []cron.JobWrapper{cron.Recover(l), cron.SkipIfStillRunning(l)}
		if p.DB != nil {
			locker := &ScheduleLoker{
				DB:     p.DB,
				Logger: p.Logger,
			}
			err := locker.Create(jobname, schedule)
			if err != nil {
				return err
			}
			opts = append(opts, locker.Wrapper())
		}
		cr := cron.New(cron.WithSeconds(), cron.WithChain(opts...))
		item, err := cr.AddFunc(schedule, cmd)
		if err != nil {
			return err
		}
		cr.Start()
		next := cr.Entry(item).Next
		p.Logger.Info("schedule job done", zap.String("job", jobname), zap.Time("next runtime", next))
		return nil
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
	// logger := zap.L()
	// l := &CronZaplog{
	// 	logger: logger,
	// }
	// cr := cron.New(cron.WithChain(cron.Recover(l), cron.SkipIfStillRunning(l)))
	// _, err := cr.AddFunc(schedule, cmd)
	// if err != nil {
	// 	return nil, err
	// }
	// cr.Start()

	// entries := cr.Entries()
	// nextRuntime := entries[0].Next

	// logger.Debug("cron job scheduled", zap.String("job", jobname), zap.String("schedule", schedule), zap.Time("next", nextRuntime))

	// return cr, nil
}
