package messaging

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

// TestConfigDefaults 缺省必须**关闭**且不裁剪任何流（安全默认：不影响未显式配置的服务）。
func TestConfigDefaults(t *testing.T) {
	viper.Reset()
	cfg := ConfigFromViper()
	if cfg.Enabled {
		t.Fatal("messaging.guard.enabled 缺省必须为 false")
	}
	if len(cfg.Topics) != 0 {
		t.Fatalf("缺省不应有白名单: %v", cfg.Topics)
	}
	if cfg.MaxBytes != DefaultMaxBytes || cfg.KeepHours != DefaultKeepHours || cfg.MinEntries != DefaultMinEntries {
		t.Fatalf("缺省预算不符: %+v", cfg)
	}
	if cfg.MinInterval != DefaultMinInterval || cfg.AlertInterval != DefaultAlertInterval {
		t.Fatalf("缺省节流/限频不符: %+v", cfg)
	}
	if !cfg.Lock.Enabled || cfg.Lock.TTL != DefaultLockTTL || cfg.Lock.KeyPrefix != DefaultLockKeyPrefix {
		t.Fatalf("缺省锁配置不符: %+v", cfg.Lock)
	}
}

// TestConfigOverride 显式配置（scm-saas 的角色）必须生效。
func TestConfigOverride(t *testing.T) {
	viper.Reset()
	viper.Set("messaging.guard.enabled", true)
	viper.Set("messaging.guard.topics", []string{"monitor.tracing", "monitor.schedule"})
	viper.Set("messaging.guard.maxBytes", int64(1024))
	viper.Set("messaging.guard.keepHours", 2)
	viper.Set("messaging.guard.minEntries", int64(10))
	viper.Set("messaging.guard.minInterval", 30*time.Second)
	viper.Set("messaging.guard.alertInterval", 5*time.Minute)
	viper.Set("messaging.guard.lock.enabled", false)

	cfg := ConfigFromViper()
	if !cfg.Enabled {
		t.Fatal("显式开启未生效")
	}
	if len(cfg.Topics) != 2 || cfg.Topics[0] != "monitor.tracing" {
		t.Fatalf("白名单未生效: %v", cfg.Topics)
	}
	if cfg.MaxBytes != 1024 || cfg.KeepHours != 2 || cfg.MinEntries != 10 {
		t.Fatalf("预算覆盖未生效: %+v", cfg)
	}
	if cfg.MinInterval != 30*time.Second || cfg.AlertInterval != 5*time.Minute {
		t.Fatalf("节流/限频覆盖未生效: %+v", cfg)
	}
	if cfg.Lock.Enabled {
		t.Fatal("锁开关覆盖未生效")
	}
}

// TestAllowTopic 白名单为空一律不放行（防止误裁未知流/业务流）。
func TestAllowTopic(t *testing.T) {
	if AllowTopic(nil, "monitor.tracing") {
		t.Fatal("空白名单必须不放行")
	}
	if AllowTopic([]string{"monitor.tracing"}, "scm.task.scanned") {
		t.Fatal("白名单外的业务流不得被裁剪")
	}
	if !AllowTopic([]string{"monitor.tracing", "monitor.error"}, "monitor.error") {
		t.Fatal("白名单内的流应放行")
	}
}

// TestTrimTargets 减半收敛：单调下降、不低于下限、轮数有上限。
func TestTrimTargets(t *testing.T) {
	got := TrimTargets(4000, 100)
	want := []int64{2000, 1000, 500, 250, 125, 100}
	if len(got) != len(want) {
		t.Fatalf("TrimTargets(4000,100) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TrimTargets(4000,100) = %v, want %v", got, want)
		}
	}
	prev := int64(4000)
	for _, target := range got {
		if target >= prev || target < 100 {
			t.Fatalf("目标序列不合法 prev=%d target=%d", prev, target)
		}
		prev = target
	}
	if v := TrimTargets(100, 100); v != nil {
		t.Fatalf("等于下限不应裁剪: %v", v)
	}
	if v := TrimTargets(0, 100); v != nil {
		t.Fatalf("空流不应裁剪: %v", v)
	}
	if v := TrimTargets(1<<40, 1); len(v) > maxTrimRounds {
		t.Fatalf("轮数 %d 超过上限 %d", len(v), maxTrimRounds)
	}
}

// TestLockKey 锁键按 topic 区分：每条流各自"一轮一个裁剪者"，互不阻塞。
func TestLockKey(t *testing.T) {
	cfg := ConfigFromViper()
	if got, want := LockKey(cfg, "monitor.tracing"), DefaultLockKeyPrefix+":monitor.tracing"; got != want {
		t.Fatalf("LockKey = %s, want %s", got, want)
	}
	cfg.Lock.KeyPrefix = "custom:prefix"
	if got, want := LockKey(cfg, "x"), "custom:prefix:x"; got != want {
		t.Fatalf("自定义前缀未生效: %s", got)
	}
	cfg.Lock.KeyPrefix = ""
	if got, want := LockKey(cfg, "x"), DefaultLockKeyPrefix+":x"; got != want {
		t.Fatalf("空前缀应回落缺省: %s", got)
	}
}
