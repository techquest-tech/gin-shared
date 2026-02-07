//go:build ram

package schedule

import (
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func CreateScheduledJob(jobname, schedule string, cmd func() error, opts ...ScheduleOptions) error {
	err := core.GetContainer().Invoke(func(logger *zap.Logger) error {
		opt := &ScheduleOptions{}
		if len(opts) > 0 {
			opt = &opts[0]
		}

		fn := wrapFuncJob(jobname, cmd, opt)

		// In RAM mode, Producer == Consumer == fn
		return startCron(jobname, schedule, fn, fn, logger, opt)
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
}

// func StartStreamWorker() {
// 	// No-op for RAM mode
// }
