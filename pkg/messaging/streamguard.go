// streamguard.go —— Redis Stream 的「字节预算」看门狗，供**发送端**在写路径上做兜底裁剪，
// 由 DefaultMessgingService.Pub 在写入成功后调用 MaybeGuard 触发。
//
// 背景（2026-10-12 事故）：单条 trace 的 body/resp 实测可达 24 MB，而 `XADD ... MAXLEN` 是
// **按条数**、且带 `denyoom` 标志的原子命令 —— 一旦实例 maxmemory 打满，写入与被裁剪会**一起失效**
// （切片裁剪和写入是同一条命令），上游静默、实例再也回不来（当天静默 111 分钟）。
//
// 为什么放在发送端：内存增长只发生在「有人往里写」的时候。消费端（monitor-adaptor）虽然也有
// 看门狗，但它是单点——消费端挂掉时上游仍在写，内存照样涨；发送端活着就必然在写，也就必然
// 有机会裁剪，等于把「守卫」和「增长源」绑在同一个生命周期上。
//
// 关键设计：
//   - **缺省关闭**（`messaging.guard.enabled=false`），只在明确配置的服务启用，避免影响全部应用；
//   - 只守白名单 topic（`topics`），**绝不碰业务流**（如 scm.task.scanned）；
//   - 每个节点**进程内节流**（`minInterval`），不是每次 Pub 都打 Redis；
//   - **多节点互斥**：每轮用 `SET NX PX` 抢锁，只让一个节点真正裁剪，避免 N 个 Pod 各裁各的
//     （裁剪结果虽然幂等，但"谁在裁、什么时候裁"会变得不可预期）；锁命令本身是 denyoom，
//     OOM 时可能失败——此时**fail-open 照常裁剪**，宁可重复也不能不裁；
//   - 内存口径 `MEMORY USAGE <key> SAMPLES 0`（遍历全部节点）：默认采样只抽 5 个节点外推，
//     对条目大小不均的流会严重低估（2026-10-12 实测 8.5 MB vs 240.6 MB，差 28 倍），低估即不触发；
//   - 裁剪顺序：先按时间（`keepHours`）→ 再按条数减半收敛到 `minEntries`（与条目大小无关）。
package messaging

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

const (
	// DefaultMaxBytes 是单条流的缺省字节预算（150 MiB）。
	DefaultMaxBytes = int64(150 * 1024 * 1024)
	// DefaultKeepHours 是第一步按时间保留的窗口。
	DefaultKeepHours = 6
	// DefaultMinEntries 是减半收敛的下限（保命优先，不再往下裁）。
	DefaultMinEntries = int64(100)
	// DefaultMinInterval 是每个节点最快的检查间隔（进程内节流）。
	DefaultMinInterval = time.Minute
	// DefaultAlertInterval 是同一条流两次上报 monitor.error 的最小间隔。
	DefaultAlertInterval = time.Hour
	// DefaultLockTTL 是抢锁的租约（应大于 MinInterval，持有者挂掉后自动释放）。
	DefaultLockTTL = 90 * time.Second
	// DefaultLockKeyPrefix 是锁键前缀，最终键为 "<prefix>:<topic>"。
	DefaultLockKeyPrefix = "messaging:guard:lock"
	// defaultGuardTimeout 限制单轮检查+裁剪的总时长，避免 goroutine 悬挂。
	defaultGuardTimeout = 30 * time.Second
	// maxTrimRounds 是减半收敛的最大轮数（4000→100 只需 6 轮）。
	maxTrimRounds = 16
)

// LockConfig 控制多节点互斥。
type LockConfig struct {
	Enabled   bool          `mapstructure:"enabled"`
	TTL       time.Duration `mapstructure:"ttl"`
	KeyPrefix string        `mapstructure:"keyPrefix"`
}

// Config 是 messaging.guard.* 配置。
type Config struct {
	Enabled       bool          `mapstructure:"enabled"`       // 总开关（**缺省 false**，按服务显式开启）
	Topics        []string      `mapstructure:"topics"`        // 白名单；留空=不裁剪任何流（安全默认）
	MaxBytes      int64         `mapstructure:"maxBytes"`      // 单流字节预算（缺省 150 MiB）
	KeepHours     int           `mapstructure:"keepHours"`     // 第一步按时间保留窗口（缺省 6h，0=跳过）
	MinEntries    int64         `mapstructure:"minEntries"`    // 减半收敛下限（缺省 100）
	MinInterval   time.Duration `mapstructure:"minInterval"`   // 进程内节流（缺省 1m）
	AlertInterval time.Duration `mapstructure:"alertInterval"` // 上报限频（缺省 1h）
	Lock          LockConfig    `mapstructure:"lock"`          // 多节点互斥
}

var (
	lastCheck sync.Map // topic -> time.Time（进程内节流）
	lastAlert sync.Map // topic -> time.Time（上报限频）
)

// ConfigFromViper 读取 messaging.guard 并套用缺省值（用 viper.Get*，便于测试与运行期覆盖）。
func ConfigFromViper() Config {
	cfg := Config{
		Enabled:       false, // 缺省关闭：只在显式配置的服务启用
		MaxBytes:      DefaultMaxBytes,
		KeepHours:     DefaultKeepHours,
		MinEntries:    DefaultMinEntries,
		MinInterval:   DefaultMinInterval,
		AlertInterval: DefaultAlertInterval,
		Lock:          LockConfig{Enabled: true, TTL: DefaultLockTTL, KeyPrefix: DefaultLockKeyPrefix},
	}
	if viper.IsSet("messaging.guard.enabled") {
		cfg.Enabled = viper.GetBool("messaging.guard.enabled")
	}
	if v := viper.GetStringSlice("messaging.guard.topics"); len(v) > 0 {
		cfg.Topics = v
	}
	if v := viper.GetInt64("messaging.guard.maxBytes"); v > 0 {
		cfg.MaxBytes = v
	}
	if viper.IsSet("messaging.guard.keepHours") {
		cfg.KeepHours = viper.GetInt("messaging.guard.keepHours")
	}
	if v := viper.GetInt64("messaging.guard.minEntries"); v > 0 {
		cfg.MinEntries = v
	}
	if v := viper.GetDuration("messaging.guard.minInterval"); v > 0 {
		cfg.MinInterval = v
	}
	if v := viper.GetDuration("messaging.guard.alertInterval"); v > 0 {
		cfg.AlertInterval = v
	}
	if viper.IsSet("messaging.guard.lock.enabled") {
		cfg.Lock.Enabled = viper.GetBool("messaging.guard.lock.enabled")
	}
	if v := viper.GetDuration("messaging.guard.lock.ttl"); v > 0 {
		cfg.Lock.TTL = v
	}
	if v := viper.GetString("messaging.guard.lock.keyPrefix"); v != "" {
		cfg.Lock.KeyPrefix = v
	}
	return cfg
}

// MaybeGuard 在写入成功后调用：做白名单与节流判断，命中则**异步**执行一轮检查+裁剪。
// 未启用、topic 不在白名单、未到节流窗口时直接返回，写路径几乎零开销。
func MaybeGuard(logger *zap.Logger, client *redis.Client, topic string) {
	if client == nil || topic == "" {
		return
	}
	cfg := ConfigFromViper()
	if !cfg.Enabled || !AllowTopic(cfg.Topics, topic) {
		return
	}
	now := time.Now()
	if v, ok := lastCheck.Load(topic); ok {
		if t, ok := v.(time.Time); ok && now.Sub(t) < cfg.MinInterval {
			return
		}
	}
	lastCheck.Store(topic, now)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("streamguard panic", zap.String("topic", topic), zap.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), defaultGuardTimeout)
		defer cancel()
		if _, err := guardOnce(ctx, logger, client, topic, cfg); err != nil {
			logger.Warn("streamguard check failed", zap.String("topic", topic), zap.Error(err))
		}
	}()
}

// guardOnce 同步执行一轮：量内存 → 抢锁 → 按时间裁 → 按条数减半收敛。返回是否发生裁剪。
func guardOnce(ctx context.Context, logger *zap.Logger, client *redis.Client, topic string, cfg Config) (bool, error) {
	mem, err := client.MemoryUsage(ctx, topic, 0).Result()
	if err != nil {
		return false, err
	}
	if mem <= cfg.MaxBytes {
		return false, nil
	}
	// 多节点互斥：拿不到锁说明已有节点在裁；锁命令报错（OOM/NOPERM）则 fail-open 继续裁。
	if cfg.Lock.Enabled {
		locked, err := acquireLock(ctx, client, cfg, topic)
		if err != nil {
			logger.Warn("streamguard lock failed, proceed anyway (fail-open)",
				zap.String("topic", topic), zap.String("lockKey", LockKey(cfg, topic)), zap.Error(err))
		} else if !locked {
			logger.Debug("streamguard skipped: another node is trimming", zap.String("topic", topic))
			return false, nil
		}
	}

	bytesBefore := mem
	entriesBefore, _ := client.XLen(ctx, topic).Result()

	trimmedByAge := int64(0)
	if cfg.KeepHours > 0 {
		minID := strconv.FormatInt(time.Now().Add(-time.Duration(cfg.KeepHours)*time.Hour).UnixMilli(), 10)
		if n, err := client.XTrimMinID(ctx, topic, minID).Result(); err != nil {
			logger.Warn("streamguard trim by age failed", zap.String("topic", topic), zap.Error(err))
		} else {
			trimmedByAge = n
		}
		mem = measure(ctx, client, topic, mem)
	}

	trimmedByLen, rounds := int64(0), 0
	targets := TrimTargets(entriesBefore, cfg.MinEntries)
	for i, target := range targets {
		if mem <= cfg.MaxBytes {
			break
		}
		n, err := client.XTrimMaxLen(ctx, topic, target).Result()
		if err != nil {
			logger.Warn("streamguard trim by len failed",
				zap.String("topic", topic), zap.Int64("target", target), zap.Error(err))
			break
		}
		trimmedByLen += n
		rounds = i + 1
		if n == 0 {
			break
		}
		mem = measure(ctx, client, topic, mem)
	}

	entriesAfter, _ := client.XLen(ctx, topic).Result()
	logger.Warn("streamguard trimmed stream",
		zap.String("topic", topic),
		zap.Int64("bytesBefore", bytesBefore), zap.Int64("bytesAfter", mem),
		zap.Int64("budget", cfg.MaxBytes),
		zap.Int64("entriesBefore", entriesBefore), zap.Int64("entriesAfter", entriesAfter),
		zap.Int64("trimmedByAge", trimmedByAge), zap.Int64("trimmedByLen", trimmedByLen),
		zap.Int("rounds", rounds), zap.Bool("withinBudget", mem <= cfg.MaxBytes))
	maybeAlert(logger, topic, mem, entriesBefore, entriesAfter, trimmedByAge+trimmedByLen, cfg)
	return true, nil
}

// acquireLock 用 SET NX PX 抢一轮的裁剪权；返回 (是否抢到, 命令错误)。
func acquireLock(ctx context.Context, client *redis.Client, cfg Config, topic string) (bool, error) {
	ttl := cfg.Lock.TTL
	if ttl <= 0 {
		ttl = DefaultLockTTL
	}
	owner := fmt.Sprintf("%d", time.Now().UnixNano())
	return client.SetNX(ctx, LockKey(cfg, topic), owner, ttl).Result()
}

// LockKey 返回某一 topic 的互斥锁键。
func LockKey(cfg Config, topic string) string {
	prefix := cfg.Lock.KeyPrefix
	if prefix == "" {
		prefix = DefaultLockKeyPrefix
	}
	return prefix + ":" + topic
}

// AllowTopic 判断 topic 是否在白名单内（白名单为空则一律不放行，防止误裁未知流）。
func AllowTopic(topics []string, topic string) bool {
	for _, t := range topics {
		if t == topic {
			return true
		}
	}
	return false
}

// TrimTargets 返回从 n 条减半收敛到 minEntries 的目标条数序列（纯函数，便于单测）。
// 例：TrimTargets(4000, 100) => [2000 1000 500 250 125 100]；n <= minEntries 时返回 nil。
func TrimTargets(n, minEntries int64) []int64 {
	var out []int64
	for n > minEntries && len(out) < maxTrimRounds {
		target := n / 2
		if target < minEntries {
			target = minEntries
		}
		if target >= n {
			break
		}
		out = append(out, target)
		n = target
	}
	return out
}

// measure 重新量一次占用；失败时回退旧值（继续按旧值决策）。
func measure(ctx context.Context, client *redis.Client, topic string, fallback int64) int64 {
	if v, err := client.MemoryUsage(ctx, topic, 0).Result(); err == nil {
		return v
	}
	return fallback
}

// maybeAlert 限频把裁剪事件上报统一告警链路（monitor.error）。
func maybeAlert(logger *zap.Logger, topic string, mem, entriesBefore, entriesAfter, trimmed int64, cfg Config) {
	if cfg.AlertInterval <= 0 {
		return
	}
	now := time.Now()
	if v, ok := lastAlert.Load(topic); ok {
		if t, ok := v.(time.Time); ok && now.Sub(t) < cfg.AlertInterval {
			return
		}
	}
	lastAlert.Store(topic, now)

	if core.ErrorAdaptor == nil {
		logger.Warn("streamguard alert skipped: ErrorAdaptor is nil", zap.String("topic", topic))
		return
	}
	msg := fmt.Sprintf("streamguard(发送端): %s 超预算已裁剪，条目 %d -> %d，剩余 %.1f MB（预算 %.1f MB），删除 %d 条；"+
		"若同一条流反复触发，说明单条 payload 过大，需在生产端做 Body/Resp 外置或截断。",
		topic, entriesBefore, entriesAfter, float64(mem)/1048576, float64(cfg.MaxBytes)/1048576, trimmed)
	core.ErrorAdaptor.Push(core.ErrorReport{
		AppName:    core.AppName,
		AppVersion: core.Version,
		Uri:        "redis.streamguard/" + topic,
		Error:      fmt.Errorf("%s", msg),
		HappendAT:  now,
	})
}
