package schedule

import (
	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var ScheduleLockerEnabled = true
var JobHistoryEnabled = true
var ScheduleDisabled = false

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

func CheckIfEnabled() cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			if ScheduleDisabled {
				zap.L().Info("cronjob is disabled.")
				return
			}
			j.Run()
		})
	}
}

func CreateSchedule(jobname, schedule string, cmd func()) error {
	err := core.GetContainer().Invoke(func(p JobParams, pp core.OptionalParam[locker.Locker]) error {
		if ScheduleDisabled {
			p.Logger.Info("cronjob is disabled.", zap.String("job", jobname))
			return nil
		}
		l := &CronZaplog{
			logger: p.Logger,
		}
		opts := []cron.JobWrapper{cron.Recover(l), cron.SkipIfStillRunning(l), CheckIfEnabled()}

		if JobHistoryEnabled && p.Bus != nil {
			opts = append(opts, Withhistory(p.Bus, jobname))
		}

		if ScheduleLockerEnabled && pp.P != nil {
			locker := &ScheduleLoker{
				Locker:  pp.P,
				Bus:     p.Bus,
				Jobname: jobname,
			}
			opts = append(opts, locker.Wrapper())
		}
		cr := cron.New(cron.WithChain(opts...))
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
}
