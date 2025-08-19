package schedule

import (
	"fmt"

	"github.com/robfig/cron/v3"
	"github.com/samber/lo"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

var ScheduleLockerEnabled = true
var JobHistoryEnabled = true
var ScheduleDisabled = false

// type CronZaplog struct {
// 	logger *zap.Logger
// }

// func (c *CronZaplog) Info(msg string, keysAndValues ...interface{}) {
// 	args := []interface{}{msg}
// 	args = append(args, keysAndValues...)
// 	c.logger.Sugar().Info(args...)
// }
// func (c *CronZaplog) Error(err error, msg string, keysAndValues ...interface{}) {
// 	args := []interface{}{err.Error() + "," + msg}
// 	args = append(args, keysAndValues...)
// 	c.logger.Sugar().Error(args...)
// }

type JobParams struct {
	dig.In
	Logger *zap.Logger
	// DB     *gorm.DB `optional:"true"`
	// Bus    EventBus.Bus `optional:"true"`
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

type ScheduleOptions struct {
	Nolocker  bool
	NoGlobal  bool // ignore ScheduleDisabled
	NoHistory bool
}

func CreateScheduledJob(jobname, schedule string, cmd func() error, opts ...ScheduleOptions) error {
	err := core.GetContainer().Invoke(func(logger *zap.Logger, locker locker.Locker) error {
		opt := &ScheduleOptions{}
		if len(opts) > 0 {
			opt = &opts[0]
		}
		if ScheduleDisabled && !opt.NoGlobal {
			logger.Info("cronjob is disabled.", zap.String("job", jobname))
			return nil
		}
		fn := wrapFuncJob(jobname, locker, cmd, opt)
		jobs[jobname] = fn
		cr := cron.New()
		item, err := cr.AddFunc(schedule, fn)
		if err != nil {
			logger.Error("add job failed", zap.Error(err))
			return err
		}
		cr.Start()
		entry := cr.Entry(item)
		next := entry.Next
		logger.Info("schedule job done", zap.String("job", jobname), zap.Time("next runtime", next))
		core.OnServiceStopping(func() {
			logger.Info("try to stop scheduled job.", zap.String("job", jobname))
			cr.Stop()
			logger.Info("scheduled job stopped.", zap.String("job", jobname))
		})
		return nil
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
}

func CreateSchedule(jobname, schedule string, cmd func(), opts ...ScheduleOptions) error {
	return CreateScheduledJob(jobname, schedule, func() error {
		cmd()
		return nil
	}, opts...)
}
