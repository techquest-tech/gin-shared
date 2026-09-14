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

// JobHistoryAdaptor is the global adaptor for job history reports.
// Defaults to the in-process chan implementation; a process can swap it for a
// redis-streaming adaptor (see messaging.NewRedisAdaptor) before services start.
var JobHistoryAdaptor core.Adaptor[JobHistory] = core.NewChanAdaptor[JobHistory](1000)

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

// parseJobHistory 解析持久化的作业历史，兼容 []byte / string 两种存储形态。
func parseJobHistory(raw any) (*JobHistory, error) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil, fmt.Errorf("unexpected job history type %T", raw)
	}

	h := &JobHistory{}
	if err := json.Unmarshal(data, h); err != nil {
		return nil, err
	}
	return h, nil
}

// GetLastDoneJobHistory 返回最近一次「真正执行完成」的作业历史；从未成功执行时返回 nil。
//
// 注意：作业注册调度时（UpsertJobSchedule）会先写入一条只含调度信息的记录，它的
// Start 是零值且 Succeed=false，并不代表作业跑过。调用方常用 Start 作为增量同步的
// 起点（如 scm-saas 的 HandleSyncWdtUnSalesStockOutInfo），一旦把这条「注册记录」
// 当成「上一次完成」，起点会被算到公元 1 年，进而构造出天量对象并撑爆内存。
func (p *JobHistoryProvider) GetLastDoneJobHistory(jobname string) *JobHistory {
	r, err := p.Persister.GetValues(context.TODO(), jobHistoryPersisterKey, jobname)
	if err != nil {
		zap.L().Error("get job history failed", zap.Error(err), zap.String("job", jobname))
		return nil
	}
	if len(r) == 0 || r[0] == nil {
		return nil
	}

	h, err := parseJobHistory(r[0])
	if err != nil {
		zap.L().Warn("parse job history failed, treat as never done",
			zap.Error(err), zap.String("job", jobname))
		return nil
	}
	if !h.Succeed || h.Start.IsZero() {
		return nil
	}

	return h
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
