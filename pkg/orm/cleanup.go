package orm

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/schedule"
	str2duration "github.com/xhit/go-str2duration/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DBCleanupReq struct {
	Cnn          string   //connection name
	Tables       []string //tables to be clean
	DeletedField string   //field name, default is deletedAt
	Duration     string   // only delete DeletedAt <  now - duration
	Batch        int      // batch per deleted
}

type CleanupService struct {
	DBMap  map[string]*gorm.DB
	logger *zap.Logger
}

func (cs *CleanupService) RegConnection(name string, db *gorm.DB) {
	if cs.DBMap == nil {
		cs.DBMap = make(map[string]*gorm.DB)
	}
	cs.DBMap[name] = db
	cs.logger.Info("registed connection", zap.String("name", name))
}

func (cs *CleanupService) GetDefaultRequest() *DBCleanupReq {
	return &DBCleanupReq{
		Batch:        10000,
		Cnn:          "default",
		DeletedField: "deleted_at",
		Duration:     "30d",
	}
}

func (cs *CleanupService) Cleanup(req *DBCleanupReq) error {
	if req.Cnn == "" {
		req.Cnn = "default"
	}
	db, ok := cs.DBMap[req.Cnn]
	if !ok {
		cs.logger.Error("db connection is not found", zap.String("cnn", req.Cnn))
		return fmt.Errorf("DB connection %s is not found", req.Cnn)
	}

	cs.logger.Info("using connection", zap.String("cnn", req.Cnn))
	duration, err := str2duration.ParseDuration(req.Duration)
	starttime := time.Now().Add(-1 * duration)
	if err != nil {
		cs.logger.Error("duration is invalid", zap.Error(err))
		return err
	}
	cs.logger.Info("duration value ", zap.Duration("duration", duration))
	for _, t := range req.Tables {
		total := int64(0)
		sql := fmt.Sprintf("delete from %s where %s <= ? limit ?", t, req.DeletedField)
		done := false
		for i := 0; !done; i++ {
			cs.logger.Info("delete row", zap.String("table", t), zap.Int("from", i*req.Batch+1), zap.Int("to", (i+1)*req.Batch))
			result := db.Session(SessionWithConfig(60*time.Second, true)).Exec(sql, starttime, req.Batch)
			if result.Error != nil {
				cs.logger.Error("delete data failed", zap.Error(result.Error))
				return fmt.Errorf("delete data failed. %v", result.Error)
			}
			cnt := result.RowsAffected
			cs.logger.Info("deleted data done", zap.Int64("rows", cnt))
			done = (cnt < int64(req.Batch))
			total = total + cnt
		}
		cs.logger.Info("done for table", zap.String("table", t), zap.Int64("total", total))
	}
	cs.logger.Info("done")
	return nil
}

func InitScheduleCleanupJob(settingkey string) interface{} {
	return func(logger *zap.Logger, cleanupService *CleanupService) core.Startup {
		settings := viper.Sub(settingkey)
		if settings == nil {
			logger.Debug("cleanup job is not scheduled")
			return nil
		}
		schedulestr := settings.GetString("schedule")
		if schedulestr == "" {
			logger.Info("cleanup job is not scheduled.")
			return nil
		}

		req := cleanupService.GetDefaultRequest()
		settings.Unmarshal(req)

		schedule.CreateSchedule("database_cleanup", schedulestr, func() {
			cleanupService.Cleanup(req)
		})

		return nil
	}
}

func DoCleanup(settingkey string, tables []string, col string, duration string) interface{} {
	return func(logger *zap.Logger, cleanupService *CleanupService) error {
		settings := viper.Sub(settingkey)
		if settings == nil {
			logger.Info("cleanup job is not scheduled.")
			return nil
		}

		req := cleanupService.GetDefaultRequest()
		settings.Unmarshal(req)
		if len(tables) > 0 {
			req.Tables = tables
		}
		if col != "" {
			req.DeletedField = col
		}
		if duration != "" {
			req.Duration = duration
		}
		err := cleanupService.Cleanup(req)
		if err != nil {
			logger.Error("clean up failed", zap.Error(err))
			return err
		}

		return nil
	}
}

func init() {
	ginshared.GetContainer().Provide(func(logger *zap.Logger, db *gorm.DB) *CleanupService {
		cs := &CleanupService{
			logger: logger,
		}
		cs.RegConnection("default", db)
		return cs
	})
	core.ProvideStartup(InitScheduleCleanupJob("cleanup"))
}
