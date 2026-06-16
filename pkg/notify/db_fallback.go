package notify

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	openDBForNotifyFn = openDBForNotify

	notifyDBMu         sync.Mutex
	cachedNotifyDB     *gorm.DB
	cachedNotifyDBDSN  string
	cachedNotifyDBPref string
)

// tryLoadFromDBOrSkip 尝试从数据库加载邮件配置；当无法连接 DB 或 DB 中不存在配置时直接跳过。
// ctx: 请求上下文，类型为 context.Context。
// 返回值：返回加载过程中的错误信息（仅在遇到非“缺失配置”类错误时返回）。
func (en *EmailNotifer) tryLoadFromDBOrSkip(ctx context.Context) error {
	if en == nil {
		return nil
	}

	namespace := strings.TrimSpace(en.namespace)
	if namespace == "" {
		namespace = guessNamespaceFromTemplates(en.Template)
		en.namespace = namespace
	}
	if strings.TrimSpace(namespace) == "" {
		return nil
	}

	dsn := strings.TrimSpace(viper.GetString("database.connection"))
	if dsn == "" {
		return nil
	}
	tablePrefix := strings.TrimSpace(viper.GetString("database.tablePrefix"))

	db, err := getNotifyDB(dsn, tablePrefix)
	if err != nil {
		if en.Logger != nil {
			en.Logger.Debug("open notify db failed, fallback to viper", zap.Error(err))
		}
		return nil
	}

	store := NewEmailNotifierStore(db)
	loaded, err := store.Load(ctx, namespace)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingNotifyTableErr(err) {
			return nil
		}
		return err
	}
	if loaded == nil || len(loaded.Template) == 0 {
		return nil
	}

	logger := en.Logger
	en.From = loaded.From
	en.SMTP = loaded.SMTP
	en.Template = loaded.Template
	en.Logger = logger
	return nil
}

func isMissingNotifyTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "error 1146") ||
		strings.Contains(msg, "no such table")
}

// getNotifyDB 获取用于读取 notify 配置的数据库连接。
// 当 DSN 或 tablePrefix 变化时会重建连接；否则复用连接池以便高频读取实时生效。
// dsn: 数据库 DSN，类型为 string；tablePrefix: 表前缀，类型为 string。
// 返回值：返回 gorm.DB 指针和错误信息。
func getNotifyDB(dsn string, tablePrefix string) (*gorm.DB, error) {
	notifyDBMu.Lock()
	defer notifyDBMu.Unlock()

	if cachedNotifyDB != nil && cachedNotifyDBDSN == dsn && cachedNotifyDBPref == tablePrefix {
		return cachedNotifyDB, nil
	}

	if cachedNotifyDB != nil {
		if sqlDB, err := cachedNotifyDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		cachedNotifyDB = nil
		cachedNotifyDBDSN = ""
		cachedNotifyDBPref = ""
	}

	db, closeFn, err := openDBForNotifyFn(dsn, tablePrefix)
	if err != nil {
		return nil, err
	}
	_ = closeFn
	cachedNotifyDB = db
	cachedNotifyDBDSN = dsn
	cachedNotifyDBPref = tablePrefix
	return cachedNotifyDB, nil
}

// openDBForNotify 打开用于读取 notify 配置的数据库连接，并返回关闭函数。
// dsn: 数据库 DSN，类型为 string；tablePrefix: 表前缀，类型为 string。
// 返回值：返回 gorm.DB、关闭函数以及错误信息。
func openDBForNotify(dsn string, tablePrefix string) (*gorm.DB, func(), error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: tablePrefix},
		PrepareStmt:    true,
	})
	if err != nil {
		return nil, func() {}, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, func() {}, err
	}
	return db, func() { _ = sqlDB.Close() }, nil
}

// guessNamespaceFromTemplates 从模板名推断 namespace。
// templates: 邮件模板集合，类型为 map[string]*EmailTmpl。
// 返回值：返回推断出的 namespace；若无法推断则返回空字符串。
func guessNamespaceFromTemplates(templates map[string]*EmailTmpl) string {
	if len(templates) == 0 {
		return ""
	}

	counter := make(map[string]int)
	for name := range templates {
		n := strings.TrimSpace(name)
		if n == "" {
			continue
		}
		parts := strings.SplitN(n, "_", 2)
		if len(parts) < 2 {
			continue
		}
		prefix := strings.TrimSpace(parts[0])
		if prefix == "" {
			continue
		}
		counter[prefix]++
	}

	best := ""
	bestCount := 0
	for k, c := range counter {
		if c > bestCount || (c == bestCount && k < best) {
			best = k
			bestCount = c
		}
	}
	return best
}
