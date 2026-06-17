package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"github.com/techquest-tech/gin-shared/pkg/cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

const (
// EventJobStarted  = "event.job.started"
// EventJobDone     = "event.job.done"
// EventJobFailed   = "event.job.failed"
// EventJobFinished = "event.job.finished"
// EventJobFailed   = "event.job.failed"
)

var JobHistoryAdaptor = core.NewChanAdaptor[JobHistory](1000)

type JobHistory struct {
	App        string
	AppVersion string
	Job        string
	Cron       string
	Start      time.Time
	Finished   time.Time
	Next       time.Time
	Duration   time.Duration
	Succeed    bool
	Message    string
	Disabled   bool
}

type JobHistoryProvider struct {
	Bus       EventBus.Bus
	Persister cache.Hash
}

var jobHistoryPersisterKey = core.AppName + ".jobs"
var provider *JobHistoryProvider

func (p *JobHistoryProvider) GetLastDoneJobHistory(jobname string) *JobHistory {
	r, err := p.Persister.GetValues(context.TODO(), jobHistoryPersisterKey, jobname)
	if err != nil {
		zap.L().Error("get job history failed", zap.Error(err), zap.String("job", jobname))
		return nil
	}
	if len(r) == 0 {
		return nil
	}

	h := &JobHistory{}
	if b, ok := r[0].([]byte); ok {
		json.Unmarshal(b, h)
		return h
	} else if s, ok := r[0].(string); ok {
		json.Unmarshal([]byte(s), h)
		return h
	}

	return nil
}
func (p *JobHistoryProvider) SetJobhistory(h JobHistory) {
	JobHistoryAdaptor.Push(h)
	data, err := json.Marshal(h)
	logger := zap.L()
	if err != nil {
		logger.Error("marshal job history failed", zap.Error(err))
		return
	}
	if h.Succeed {
		p.Persister.SetValues(context.TODO(), jobHistoryPersisterKey, map[string]any{h.Job: string(data)})
	} else {
		logger.Warn("job is not succeed, ignore history")
	}

}

func (p *JobHistoryProvider) UpsertJobSchedule(jobname, schedule string, next time.Time, disabled bool) {
	var h *JobHistory

	r, err := p.Persister.GetValues(context.TODO(), jobHistoryPersisterKey, jobname)
	if err == nil && len(r) > 0 && r[0] != nil {
		h = &JobHistory{}
		switch v := r[0].(type) {
		case []byte:
			if err := json.Unmarshal(v, h); err != nil {
				h = nil
			}
		case string:
			if err := json.Unmarshal([]byte(v), h); err != nil {
				h = nil
			}
		default:
			h = nil
		}
	}

	if h == nil {
		h = &JobHistory{}
	}

	h.App = core.AppName
	h.AppVersion = core.Version
	h.Job = jobname
	h.Cron = schedule
	h.Next = next
	h.Disabled = disabled

	data, err := json.Marshal(h)
	if err != nil {
		zap.L().Error("marshal job history failed", zap.Error(err))
		return
	}
	if err := p.Persister.SetValues(context.TODO(), jobHistoryPersisterKey, map[string]any{jobname: string(data)}); err != nil {
		zap.L().Error("set job history failed", zap.Error(err), zap.String("job", jobname))
		return
	}
}
func init() {
	core.Provide(func(bus EventBus.Bus, h cache.Hash) *JobHistoryProvider {
		return &JobHistoryProvider{Bus: bus, Persister: h}
	})
	core.ProvideStartup(func(p *JobHistoryProvider) core.Startup {
		provider = p
		return nil
	})
}

func GetLastDoneJobHistory(jobname string) *JobHistory {
	if provider != nil {
		return provider.GetLastDoneJobHistory(jobname)
	}
	zap.L().Warn("job history provider not initialized")
	return nil
}

// decrepted, will be removed next release.
func Withhistory(jobname string) cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(
			func() {
				logger := zap.L().With(zap.String("jobname", jobname))
				logger.Debug("mark job started")
				task := JobHistory{
					App:        core.AppName,
					AppVersion: core.Version,
					Job:        jobname,
					Cron:       resolveJobSchedule(jobname, ""),
					Start:      time.Now(),
					Succeed:    true,
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
					} else {
						logger.Info("job done")
					}

					done := time.Now()
					task.Duration = time.Since(task.Start)
					task.Finished = done
					task.Next = resolveJobNextRuntime(jobname, task.Cron, done)
					logger.Debug("job end", zap.Duration("duration", task.Duration))

					if provider != nil {
						provider.SetJobhistory(task)
					}
				}()
				j.Run()
			})
	}
}
