package schedule

import (
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

const (
	// EventJobStarted  = "event.job.started"
	// EventJobDone     = "event.job.done"
	// EventJobFailed   = "event.job.failed"
	EventJobFinished = "event.job.finished"
	EventJobFailed   = "event.job.failed"
)

type JobHistory struct {
	Job      string
	Start    time.Time
	Finished time.Time
	Duration time.Duration
	Succeed  bool
	Message  string
}

func Withhistory(bus EventBus.Bus, jobname string) cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(
			func() {
				logger := zap.L().With(zap.String("jobname", jobname))
				logger.Debug("mark job started")
				task := JobHistory{
					Job:     jobname,
					Start:   time.Now(),
					Succeed: true,
				}
				defer func() {
					if r := recover(); r != nil {
						if err, ok := r.(error); ok {
							task.Message = err.Error()
						} else if msg, ok := r.(string); ok {
							task.Message = msg
						}
						task.Succeed = false
						logger.Error("recover from panic", zap.Any("panic", r), zap.String("job", task.Job))
					}
					logger.Info("job done")

					done := time.Now()
					task.Duration = time.Since(task.Start)
					task.Finished = done
					logger.Debug("mark job end", zap.Duration("duration", task.Duration))
					if bus != nil {
						bus.Publish(EventJobFinished, task)
					}
				}()

				j.Run()
			})
	}
}
