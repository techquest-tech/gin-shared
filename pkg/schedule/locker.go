package schedule

import (
	"context"
	"errors"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

var LockerTimeout = 30 *time.Second//time.Hour

type ScheduleLoker struct {
	Locker  locker.Locker
	Jobname string
}

var errlockerFailed = errors.New("get locker failed")

func (sl *ScheduleLoker) Wrapper() cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			ctx := context.TODO()
			// startAt := time.Now()
			logger := zap.L().With(zap.String("Jobname", sl.Jobname))
			// logger.Debug("try to get locker", zap.String("locker", fmt.Sprintf("%T", sl.Locker)))
			release, err := sl.Locker.LockWithtimeout(ctx, sl.Jobname, LockerTimeout)
			if err != nil {
				logger.Info("get locker failed. job cancel.", zap.Error(err))
				// panic(errlockerFailed)
				return
			}
			defer release(ctx)
			// logger.Debug("got locker")
			j.Run()
			// dur := time.Since(startAt)
			// logger.Info("job done.", zap.Duration("duration", dur))
		})
	}
}

func init() {
	core.ProvideStartup(func(logger *zap.Logger) core.Startup {
		dur := viper.GetDuration("locker.timeout")
		if dur > 0 {
			LockerTimeout = dur
			logger.Info("set default cronjob locker timeout", zap.Duration("locker", LockerTimeout))
		}
		return nil
	})
}
