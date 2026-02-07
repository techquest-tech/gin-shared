package schedule

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/samber/lo"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

var ScheduleLockerEnabled = true
var JobHistoryEnabled = true
var ScheduleDisabled = false
var JobRecoveryEnabled = true

const (
	StreamKey   = "gin-shared:jobs:stream"
	GroupKey    = "gin-shared:jobs:group"
	ConsumerKey = "consumer"
)

func CheckIfEnabled() cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			if ScheduleDisabled {
				zap.L().Info("cronjob is disabled.")
				return
			}
			j.Run()
		})
	}
}

var jobs = make(map[string]func())

func Run(jobname string) (err error) {
	rawSetting := ScheduleDisabled
	ScheduleDisabled = true
	fn, ok := jobs[jobname]
	defer func() {
		ScheduleDisabled = rawSetting
		if err := recover(); err != nil {
			zap.L().Error("run job failed", zap.String("job", jobname), zap.Any("err", err))
		}
	}()
	if ok {
		zap.L().Info("run job", zap.String("job", jobname))
		fn()
		return nil
	}
	return fmt.Errorf("job %s not found", jobname)
}

func List() []string {
	return lo.Keys(jobs)
}

type ScheduleOptions struct {
	Nolocker  bool
	NoGlobal  bool // ignore ScheduleDisabled
	NoHistory bool
}

type ScheduledJob struct {
	Cron     *cron.Cron
	EntryID  cron.EntryID
	Name     string
	Schedule string
	Fn       func()
}

var (
	scheduledJobs = make(map[string]*ScheduledJob)
	jobMux        sync.RWMutex
)

func CreateScheduledJobWithContext(jobname, schedule string, cmd func(ctx context.Context) error, opts ...ScheduleOptions) error {
	fn := func() error {
		ctx := context.Background()
		err := cmd(ctx)
		if err != nil {
			zap.L().Error("run job failed", zap.String("job", jobname), zap.Error(err))
		}
		return err
	}
	return CreateScheduledJob(jobname, schedule, fn, opts...)
}

func CreateSchedule(jobname, schedule string, cmd func(), opts ...ScheduleOptions) error {
	return CreateScheduledJob(jobname, schedule, func() error {
		cmd()
		return nil
	}, opts...)
}

// startCron is a helper to start the cron job.
// It assumes producerFn is the function to be scheduled by Cron (trigger).
// It assumes consumerFn is the function to be executed (logic), stored in jobs map.
func startCron(jobname, schedule string, producerFn func(), consumerFn func(), logger *zap.Logger, opt *ScheduleOptions) error {
	jobs[jobname] = consumerFn

	if (ScheduleDisabled && !opt.NoGlobal) || schedule == "" || schedule == "-" {
		logger.Info("cronjob is disabled.", zap.String("job", jobname))
		return nil
	}

	chain := []cron.JobWrapper{}
	if !opt.NoGlobal {
		chain = append(chain, CheckIfEnabled())
	}
	// found cron won't work with sub-second scheduling, use time.Ticker instead
	if durStr, ok := strings.CutPrefix(schedule, "@every "); ok {
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			logger.Warn("parse duration failed, continue with cron", zap.Error(err), zap.String("dur", durStr))
		}
		if dur < time.Second {
			logger.Info("use time.Ticker instead of cron", zap.Duration("dur", dur))
			ticker := time.NewTicker(dur)
			core.OnServiceStopping(func() {
				zap.L().Info("ticker stopped", zap.String("job", jobname))
				ticker.Stop()
			})
			go func() {
				for range ticker.C {
					zap.L().Debug("ticker ticking", zap.String("job", jobname), zap.Duration("duration", dur))
					producerFn()
					zap.L().Debug("ticker ticked", zap.String("job", jobname))
				}
			}()
			return nil
		}
	}

	cr := cron.New(cron.WithChain(chain...))

	item, err := cr.AddFunc(schedule, producerFn)
	if err != nil {
		logger.Error("add job failed", zap.Error(err))
		return err
	}
	cr.Start()

	jobMux.Lock()
	scheduledJobs[jobname] = &ScheduledJob{
		Cron:     cr,
		EntryID:  item,
		Name:     jobname,
		Schedule: schedule,
		Fn:       producerFn,
	}
	jobMux.Unlock()

	entry := cr.Entry(item)
	next := entry.Next
	logger.Info("schedule job done", zap.String("job", jobname), zap.String("schedule", schedule), zap.Time("next runtime", next))
	core.OnServiceStopping(func() {
		logger.Info("try to stop scheduled job.", zap.String("job", jobname))
		cr.Stop()
		logger.Info("scheduled job stopped.", zap.String("job", jobname))
	})
	return nil
}
