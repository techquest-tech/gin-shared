package schedule

import (
	"fmt"
	"runtime"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

func wrapFuncJob(jobname string, lockerService locker.Locker, fn func() error, opt *ScheduleOptions) cron.FuncJob {
	return cron.FuncJob(
		func() {
			logger := zap.L().With(zap.String("jobname", jobname))
			task := JobHistory{
				App:     core.AppName,
				Job:     jobname,
				Start:   time.Now(),
				Succeed: true,
			}
			ctx := core.RootCtx()
			var release locker.Release
			var err error
			if !opt.Nolocker {
				release, err = lockerService.WaitForLocker(ctx, jobname, LockerTimeout, LockerTimeout)
				if err != nil {
					logger.Warn("get locker failed. another job is running", zap.Error(err))
					task.Succeed = false
					task.Message = "Duplicated job"
					if provider != nil && !opt.NoHistory {
						provider.SetJobhistory(task)
					}
					return
				}
			}

			defer func() {
				if release != nil {
					release(ctx)
				}

				if r := recover(); r != nil {
					if err, ok := r.(error); ok {
						// if errors.Is(err, errlockerFailed) {
						// 	logger.Info("job cancelled, another job might be running", zap.Error(err))
						// 	return
						// } else {
						task.Message = err.Error()
						logger.Error("recover from panic", zap.Error(err), zap.String("job", task.Job))
						// }
					} else if msg, ok := r.(string); ok {
						task.Message = msg
						logger.Info(msg)
					} else {
						task.Message = fmt.Sprintf("unknown error: %v", r) // should never happen
						logger.Error("recover from panic", zap.Any("job", task.Job))
					}
					task.Succeed = false

					buf := make([]byte, 1024)
					for {
						n := runtime.Stack(buf, true)
						if n < len(buf) {
							buf = buf[:n]
							break
						}
						buf = make([]byte, 2*len(buf))
					}
					fmt.Printf("Full stack trace:\n%s", buf)
				}

				if task.Succeed {
					logger.Info("job done")
				}

				done := time.Now()
				task.Duration = time.Since(task.Start)
				task.Finished = done
				logger.Debug("job end", zap.Duration("duration", task.Duration))
				if provider != nil && !opt.NoHistory {
					provider.SetJobhistory(task)
				}
			}()
			err = fn()
			if err != nil {
				logger.Error("run job failed", zap.Error(err))
				task.Succeed = false
				task.Message = err.Error()
			}
		})
}
