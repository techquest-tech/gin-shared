package schedule

import (
	"context"
	"fmt"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

var LockerTimeout = 50 * time.Millisecond

type ScheduleLoker struct {
	Locker  locker.Locker
	Bus     EventBus.Bus
	Jobname string
}

func (sl *ScheduleLoker) Wrapper() cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			ctx := context.TODO()
			// startAt := time.Now()
			logger := zap.L().With(zap.String("Jobname", sl.Jobname))
			logger.Debug("try to get locker", zap.String("locker", fmt.Sprintf("%T", sl.Locker)))
			release, err := sl.Locker.LockWithtimeout(ctx, sl.Jobname, LockerTimeout)
			if err != nil {
				logger.Info("get locker failed. job cancel.", zap.Error(err))
				// return
				panic("get locker failed. job cancel.")
			}
			defer release(ctx)
			logger.Debug("got locker")
			j.Run()
			// dur := time.Since(startAt)
			// logger.Info("job done.", zap.Duration("duration", dur))
		})
	}
}
