package schedule

import (
	"fmt"
	"runtime"
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
				task := &JobHistory{
					Job:     jobname,
					Start:   time.Now(),
					Succeed: true,
				}
				defer func() {
					if r := recover(); r != nil {
						if err, ok := r.(error); ok {
							task.Message = err.Error()
							logger.Error("recover from panic", zap.Error(err), zap.String("job", task.Job))
						} else if msg, ok := r.(string); ok {
							task.Message = msg
							logger.Info(msg)
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
					logger.Info("job done")

					done := time.Now()
					task.Duration = time.Since(task.Start)
					task.Finished = done
					logger.Debug("job end", zap.Duration("duration", task.Duration))
					if bus != nil {
						bus.Publish(EventJobFinished, task)
					}
				}()

				j.Run()
			})
	}
}
