package orm

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// captureAdaptor 收集推送到 error 通道的上报，供断言使用。
// 不带 TryPush，覆盖 reportInitException 的 Push 回退分支。
type captureAdaptor struct {
	mu  sync.Mutex
	got []core.ErrorReport
}

func (c *captureAdaptor) Push(data core.ErrorReport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, data)
}

func (c *captureAdaptor) Sub(receiver string) <-chan core.ErrorReport { return nil }
func (c *captureAdaptor) Subscripter(receiver string, fn core.Handler[core.ErrorReport]) {
}
func (c *captureAdaptor) Start()              {}
func (c *captureAdaptor) Stop()               {}
func (c *captureAdaptor) Receivers() []string { return nil }
func (c *captureAdaptor) reports() []core.ErrorReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]core.ErrorReport, len(c.got))
	copy(out, c.got)
	return out
}

// tryCaptureAdaptor 实现 TryPusher，覆盖 reportInitException 的非阻塞分支。
type tryCaptureAdaptor struct {
	*captureAdaptor
	saturated bool
	tryCalls  int
}

func (t *tryCaptureAdaptor) TryPush(data core.ErrorReport) bool {
	t.tryCalls++
	if t.saturated {
		return false
	}
	t.Push(data)
	return true
}

// swapErrorAdaptor 把全局 error 通道换成测试用 adaptor，并在用例结束后还原。
func swapErrorAdaptor(t *testing.T, adaptor core.Adaptor[core.ErrorReport]) {
	t.Helper()
	old := core.ErrorAdaptor
	core.ErrorAdaptor = adaptor
	t.Cleanup(func() { core.ErrorAdaptor = old })
}

func TestReportInitExceptionPushesToErrorChannel(t *testing.T) {
	capture := &captureAdaptor{}
	swapErrorAdaptor(t, capture)

	// nil error 不上报
	reportInitException(nil, InitDBUri, nil)
	if len(capture.reports()) != 0 {
		t.Fatalf("nil error must not be reported, got %d", len(capture.reports()))
	}

	boom := errors.New("migrate boom")
	reportInitException(boom, InitDBUri+"/migrateTables", nil)

	reports := capture.reports()
	if len(reports) != 1 {
		t.Fatalf("expect 1 report, got %d", len(reports))
	}
	rr := reports[0]
	if rr.Error == nil || !strings.Contains(rr.Error.Error(), "migrate boom") {
		t.Errorf("unexpected error text: %v", rr.Error)
	}
	if rr.Uri != InitDBUri+"/migrateTables" {
		t.Errorf("unexpected uri: %s", rr.Uri)
	}
	if rr.AppName != core.AppName || rr.AppVersion != core.Version {
		t.Errorf("app info not filled: %s/%s", rr.AppName, rr.AppVersion)
	}
	if len(rr.FullStack) == 0 {
		t.Error("expect auto captured stack")
	}
	if rr.HappendAT.IsZero() {
		t.Error("expect happened time")
	}
}

func TestReportInitExceptionUsesTryPushWhenAvailable(t *testing.T) {
	capture := &tryCaptureAdaptor{captureAdaptor: &captureAdaptor{}}
	swapErrorAdaptor(t, capture)

	reportInitException(errors.New("views boom"), InitDBUri+"/initViews", []byte("given stack"))

	if capture.tryCalls != 1 {
		t.Fatalf("expect TryPush used once, got %d", capture.tryCalls)
	}
	reports := capture.reports()
	if len(reports) != 1 {
		t.Fatalf("expect 1 report, got %d", len(reports))
	}
	if string(reports[0].FullStack) != "given stack" {
		t.Errorf("caller stack must be kept, got %q", string(reports[0].FullStack))
	}

	// 通道打满：TryPush 返回 false，只丢这一条，不能 panic。
	capture.saturated = true
	reportInitException(errors.New("again"), InitDBUri, nil)
	if len(capture.reports()) != 1 {
		t.Errorf("saturated channel should drop the report, got %d", len(capture.reports()))
	}
}

// stubDialector 让 MigrateTableAndView 能在不连任何真实库的情况下跑完流程
// （entities 为空，AutoMigrate 走 migrator stub）。
type stubDialector struct{ gorm.Dialector }

func (d *stubDialector) Name() string              { return "stubdb" }
func (d *stubDialector) Initialize(*gorm.DB) error { return nil }

func (d *stubDialector) Migrator(*gorm.DB) gorm.Migrator                       { return &stubMigrator{} }
func (d *stubDialector) DataTypeOf(*schema.Field) string                       { return "" }
func (d *stubDialector) DefaultValueOf(*schema.Field) clause.Expression        { return nil }
func (d *stubDialector) BindVarTo(clause.Writer, *gorm.Statement, interface{}) {}

func (d *stubDialector) QuoteTo(clause.Writer, string) {}

func (d *stubDialector) Explain(sql string, vars ...interface{}) string { return sql }

type stubMigrator struct{ gorm.Migrator }

func (m *stubMigrator) AutoMigrate(dst ...interface{}) error { return nil }

func TestMigrateTableAndViewReportsPostMigrateFailure(t *testing.T) {
	capture := &captureAdaptor{}
	swapErrorAdaptor(t, capture)

	oldInitDB := viper.GetBool(KeyInitDB)
	t.Cleanup(func() { viper.Set(KeyInitDB, oldInitDB) })
	viper.Set("database.type", "stubdb")
	t.Cleanup(func() { viper.Set("database.type", "mysql") })

	boom := errors.New("post migrate boom")
	RegisterPostMigrate(func(db *gorm.DB, logger *zap.Logger) error { return boom })
	t.Cleanup(func() {
		postMigrateMu.Lock()
		postMigrateHooks = postMigrateHooks[:0]
		postMigrateMu.Unlock()
	})

	db, err := gorm.Open(&stubDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open stub db: %v", err)
	}

	MigrateTableAndView(db, zap.NewNop(), nil)

	reports := capture.reports()
	if len(reports) != 1 {
		t.Fatalf("expect 1 report for post migrate failure, got %d", len(reports))
	}
	if reports[0].Uri != InitDBUri+"/postMigrate" {
		t.Errorf("unexpected uri: %s", reports[0].Uri)
	}
	if !errors.Is(reports[0].Error, boom) {
		t.Errorf("unexpected error: %v", reports[0].Error)
	}
}

func TestRunInitDBWithReportRecoversPanic(t *testing.T) {
	capture := &tryCaptureAdaptor{captureAdaptor: &captureAdaptor{}}
	swapErrorAdaptor(t, capture)

	// orm.InitDB 连不上库时就是这个形态：panic 直接冒到命令层。
	err := runInitDBWithReport(func() error { panic("connect to db failed") })
	if err == nil {
		t.Fatal("expect the panic to be converted into an error")
	}
	if !strings.Contains(err.Error(), "connect to db failed") {
		t.Errorf("unexpected error: %v", err)
	}

	reports := capture.reports()
	if len(reports) != 1 {
		t.Fatalf("expect 1 report, got %d", len(reports))
	}
	if reports[0].Uri != InitDBUri+"/panic" {
		t.Errorf("unexpected uri: %s", reports[0].Uri)
	}
	if len(reports[0].FullStack) == 0 {
		t.Error("expect panic stack in report")
	}
}

func TestRunInitDBWithReportReportsReturnedError(t *testing.T) {
	capture := &tryCaptureAdaptor{captureAdaptor: &captureAdaptor{}}
	swapErrorAdaptor(t, capture)

	boom := errors.New("invoke failed")
	err := runInitDBWithReport(func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("expect error returned unchanged, got %v", err)
	}

	reports := capture.reports()
	if len(reports) != 1 {
		t.Fatalf("expect 1 report, got %d", len(reports))
	}
	if reports[0].Uri != InitDBUri {
		t.Errorf("unexpected uri: %s", reports[0].Uri)
	}
	if !errors.Is(reports[0].Error, boom) {
		t.Errorf("unexpected reported error: %v", reports[0].Error)
	}
}

// 端到端兜底：initDB 命令本身（容器依赖解析失败也应上报 error 通道）。
func TestInitDBCmdReportsExceptionToErrorChannel(t *testing.T) {
	capture := &tryCaptureAdaptor{captureAdaptor: &captureAdaptor{}}
	swapErrorAdaptor(t, capture)

	err := InitDBCmd.RunE(InitDBCmd, nil)
	if err == nil {
		t.Fatal("expect initDB to return an error")
	}

	reports := capture.reports()
	if len(reports) == 0 {
		t.Fatal("expect initDB exception reported to error channel")
	}
	if !strings.HasPrefix(reports[0].Uri, InitDBUri) {
		t.Errorf("unexpected uri: %s", reports[0].Uri)
	}
	if reports[0].Error == nil {
		t.Error("expect error text in report")
	}
}
