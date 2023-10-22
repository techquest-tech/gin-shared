package schedule

import (
	"errors"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// It's for job locker
type JobSchedule struct {
	Name         string `gorm:"size:64;primarykey"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Schedule     string `gorm:"size:32"`
	LastRuntime  time.Time
	LastDuration time.Duration
}

type ScheduleLoker struct {
	Logger  *zap.Logger
	DB      *gorm.DB
	Bus     EventBus.Bus
	Jobname string
}

func (sl *ScheduleLoker) Create(name, schedule string) error {
	sl.Jobname = name
	sh := &JobSchedule{
		Name:     name,
		Schedule: schedule,
	}
	err := sl.DB.First(sh, "name = ? ", name).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if sh.CreatedAt.IsZero() {
				sh.CreatedAt = time.Now()
			}
			if sh.UpdatedAt.IsZero() {
				sh.UpdatedAt = time.Now()
			}
			if sh.LastRuntime.IsZero() {
				sh.LastRuntime = time.Now()
			}
			err = sl.DB.Save(sh).Error
			if err != nil {
				sl.Logger.Error("save job record failed.")
				return err
			}
			sl.Logger.Info("save job records done")
		}
		sl.Logger.Error("query record failed.")
		return err
	}

	if sh.Schedule != schedule {
		sh.Schedule = schedule
		sl.DB.Save(sh)
	}
	return nil
}

func (sl *ScheduleLoker) Wrapper() cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			sl.DB.Transaction(func(tx *gorm.DB) error {
				startAt := time.Now()
				sl.Logger.Info("start run job", zap.String("job", sl.Jobname))
				err := tx.Model(&JobSchedule{}).Where("name = ?", sl.Jobname).Update("last_runtime", startAt).Error
				if err != nil {
					sl.Logger.Info("get locker failed, might be another Job is running.", zap.Error(err))
					return nil
				}
				dur := time.Since(startAt)
				if dur > time.Millisecond*50 {
					sl.Logger.Warn("get locker > 1 second, might be another job is running.")
					return nil
				}
				j.Run()
				dur = time.Since(startAt)
				err = tx.Model(&JobSchedule{}).Where("name = ?", sl.Jobname).Update("last_duration", dur).Error
				if err != nil {
					sl.Logger.Warn("update job duration failed.", zap.Error(err))
				}
				sl.Logger.Info("job done", zap.String("job", sl.Jobname), zap.Duration("duration", dur))
				return nil
			})
		})
	}
}
