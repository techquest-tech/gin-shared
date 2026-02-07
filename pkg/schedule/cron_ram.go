//go:build ram

package schedule

import (
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

func CreateScheduledJob(jobname, schedule string, cmd func() error, opts ...ScheduleOptions) error {
	err := core.GetContainer().Invoke(func(logger *zap.Logger) error {
		opt := &ScheduleOptions{}
		if len(opts) > 0 {
			opt = &opts[0]
		}

		// Force Nolocker in RAM mode as requested
		opt.Nolocker = true

		// We pass nil for locker service because Nolocker is true, so it won't be used.
		var lockerService locker.Locker = nil

		fn := wrapFuncJob(jobname, lockerService, cmd, opt)

		// In RAM mode, Producer == Consumer == fn
		return startCron(jobname, schedule, fn, fn, logger, opt)
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
}

func StartStreamWorker() {
	// No-op for RAM mode
}
