package schedule

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func resolveJobSchedule(jobname, schedule string) string {
	if schedule != "" {
		return schedule
	}

	jobMux.RLock()
	sj := scheduledJobs[jobname]
	jobMux.RUnlock()
	if sj == nil {
		return ""
	}
	return sj.Schedule
}

func resolveJobNextRuntime(jobname, schedule string, finishedAt time.Time) time.Time {
	jobMux.RLock()
	sj := scheduledJobs[jobname]
	jobMux.RUnlock()
	if sj != nil && sj.Cron != nil {
		entry := sj.Cron.Entry(sj.EntryID)
		if !entry.Next.IsZero() {
			return entry.Next
		}
	}

	if durStr, ok := strings.CutPrefix(schedule, "@every "); ok {
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return time.Time{}
		}
		return finishedAt.Add(dur)
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}
	}
	return sched.Next(finishedAt)
}

func wrapFuncJob(jobname, schedule string, fn func() error, opt *ScheduleOptions) cron.FuncJob {
	return cron.FuncJob(
		func() {
			logger := zap.L().With(zap.String("jobname", jobname))
			task := JobHistory{
				App:        core.AppName,
				AppVersion: core.Version,
				Job:        jobname,
				Cron:       resolveJobSchedule(jobname, schedule),
				Start:      time.Now(),
				Succeed:    true,
			}
			var err error

			defer func() {
				if r := recover(); r != nil {
					if err, ok := r.(error); ok {
						task.Message = err.Error()
						logger.Error("recover from panic", zap.Error(err), zap.String("job", task.Job))
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
				task.Next = resolveJobNextRuntime(jobname, task.Cron, done)
				logger.Debug("job end", zap.Duration("duration", task.Duration))
				if provider != nil && !opt.NoHistory {
					provider.SetJobhistory(task)
				}
			}()
			for i := 0; i <= opt.RetryTimes; i++ {
				err = fn()
				if err == nil {
					break
				}

				if i < opt.RetryTimes {
					logger.Warn("job failed, retrying...", zap.Error(err), zap.Int("attempt", i+1), zap.Int("max_retry", opt.RetryTimes))
					if opt.RetryWait > 0 {
						time.Sleep(opt.RetryWait)
					}
				} else {
					logger.Error("run job failed", zap.Error(err))
					task.Succeed = false
					task.Message = err.Error()
				}
			}
		})
}
