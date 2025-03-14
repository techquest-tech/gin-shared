package schedule

import (
	"fmt"

	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"github.com/samber/lo"
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

var jobs = make(map[string]func())

func Run(jobname string) (err error) {
	ScheduleDisabled = true
	fn, ok := jobs[jobname]
	defer func() {
		ScheduleDisabled = false
		if err := recover(); err != nil {
			zap.L().Error("run job failed", zap.String("job", jobname), zap.Any("err", err))
		}
	}()
	if ok {
		zap.L().Info("run job", zap.String("job", jobname))
		fn()
		return nil
	}
	return fmt.Errorf("job %s not found", jobname)
}

func List() []string {
	return lo.Keys(jobs)
}

func CreateSchedule(jobname, schedule string, cmd func()) error {
	jobs[jobname] = cmd

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
			opts = append(opts, Withhistory(jobname))
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
		entry := cr.Entry(item)
		next := entry.Next
		p.Logger.Info("schedule job done", zap.String("job", jobname), zap.Time("next runtime", next))
		core.OnServiceStopping(func() {
			p.Logger.Info("try to stop scheduled job.", zap.String("job", jobname))
			cr.Stop()
			p.Logger.Info("scheduled job stopped.", zap.String("job", jobname))
		})
		return nil
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
}
