package orm

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// InitDBUri 是 initDB 阶段上报到 error 通道时使用的 "uri"。
// initDB 不是 http 请求，用命令名代替，方便在错误上报里按环节区分；
// 各失败环节以 "initDB/xxx" 的形式上报。
const InitDBUri = "initDB"

// reportInitException 把 initDB 阶段的异常推送到全局 error 通道
// （core.ErrorAdaptor，由 monitor 消费后写入 monitor.error / DuckDB 错误上报）。
//
// initDB 既没有 gin 中间件兜底、也不一定有调用方检查返回值，只写日志的问题
// 往往没人看见，因此这里在 logger.Error 之外再做一次显式上报：
//   - uri 标识失败环节（如 "initDB/migrateTables"）；
//   - stack 为 nil 时自动抓取当前 goroutine 栈，panic 场景由调用方传入
//     debug.Stack()；
//   - 上报走非阻塞 TryPush（通道打满只丢这一条并打日志），不让上报本身把
//     initDB 卡住。
func reportInitException(err error, uri string, stack []byte) {
	if err == nil {
		return
	}
	if len(stack) == 0 {
		stack = debug.Stack()
	}
	rr := core.ErrorReport{
		AppName:    core.AppName,
		AppVersion: core.Version,
		Uri:        uri,
		FullStack:  stack,
		Error:      err,
		HappendAT:  time.Now(),
	}
	if pusher, ok := core.ErrorAdaptor.(core.TryPusher[core.ErrorReport]); ok {
		if !pusher.TryPush(rr) {
			zap.L().Warn("initDB error report dropped, error channel saturated",
				zap.String("uri", uri), zap.Error(err))
		}
		return
	}
	core.ErrorAdaptor.Push(rr)
}

var entities = make([]interface{}, 0)

func AppendEntity(entity ...interface{}) {
	entities = append(entities, entity...)
}

var postMigrateMu sync.Mutex
var postMigrateHooks = make([]func(db *gorm.DB, logger *zap.Logger) error, 0)

func RegisterPostMigrate(fn func(db *gorm.DB, logger *zap.Logger) error) {
	if fn == nil {
		return
	}
	postMigrateMu.Lock()
	postMigrateHooks = append(postMigrateHooks, fn)
	postMigrateMu.Unlock()
}

var ormOnce sync.Once

var migrateFN = func(db *gorm.DB, logger *zap.Logger, bus core.OptionalParam[EventBus.Bus]) {
	ormOnce.Do(func() {
		if viper.GetBool(KeyInitDB) {
			MigrateTableAndView(db, logger, bus.P)
		}
	})
}

func ApplyMigrate() error {
	return core.GetContainer().Invoke(migrateFN)
}

func MigrateTableAndView(db *gorm.DB, logger *zap.Logger, bus EventBus.Bus, cleanViews ...string) {
	dialect := ""
	if db != nil && db.Dialector != nil {
		dialect = db.Dialector.Name()
	}

	viewsToClean := cleanViews
	if dialect == "postgres" && len(viewsToClean) == 0 {
		viewsToClean = []string{"*"}
	}
	if len(viewsToClean) > 0 {
		err := CleanViews(db, logger, viewsToClean)
		if err != nil {
			logger.Error("clean views failed", zap.Error(err))
			reportInitException(err, InitDBUri+"/cleanViews", nil)
		}
	}

	logger.Info("init all tables")

	for _, item := range entities {
		name := fmt.Sprintf("%T", item)
		zap.L().Info("applied entity", zap.String("entity", name))
	}
	err := db.AutoMigrate(entities...)
	if err != nil {
		logger.Error("init tables failed", zap.Error(err))
		reportInitException(err, InitDBUri+"/migrateTables", nil)
	} else {
		postMigrateMu.Lock()
		hooks := make([]func(db *gorm.DB, logger *zap.Logger) error, len(postMigrateHooks))
		copy(hooks, postMigrateHooks)
		postMigrateMu.Unlock()
		for _, hook := range hooks {
			if hook == nil {
				continue
			}
			if err := hook(db, logger); err != nil {
				logger.Error("post migrate hook failed", zap.Error(err))
				reportInitException(err, InitDBUri+"/postMigrate", nil)
			}
		}
		if bus != nil {
			bus.Publish("sys.db.inited")
		}
		logger.Info("init tables done")
	}

	err = InitMysqlViews(db, logger)
	if err != nil {
		logger.Error("init views failed", zap.Error(err))
		reportInitException(err, InitDBUri+"/initViews", nil)
	} else {
		logger.Info("init views done.")
	}
}

func init() {
	// TODO: should check if db if empty, might error when service bootup, should init when connect to DB.
	core.InvokeAsyncOnServiceStarted(migrateFN)
}
