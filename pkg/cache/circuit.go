package cache

import (
	"errors"
	"sync"
	"time"
)

// ErrCacheUnavailable 表示缓存后端（Redis）处于熔断打开状态，本次操作已快速失败、未真正访问后端。
// 调用方应把它等同于「缓存未命中 / 缓存不可用」，回退到数据源（DB 等），不要因此中断主业务。
var ErrCacheUnavailable = errors.New("cache: circuit open, backend temporarily unavailable")

// healthGate 是缓存后端的轻量熔断器。
//
// 背景：Redis 在启动时已通过 Ping 校验，但运行期仍可能宕机/抖动。go-redis 默认
// 有 DialTimeout(5s)+ReadTimeout(3s)+MaxRetries(3) 的重试，一旦后端不可用，每个缓存操作
// 都会阻塞数秒；而 DisplayName 这类流程一次会连续触发多次缓存读写（GetValues/SetValues/
// Existed…），叠加「缓存未命中→回源 DB→回写缓存」的放大路径，单个请求会被拖到十秒级，
// 高并发下迅速打满 DB 与请求队列。
//
// 熔断器在「一次读失败」后进入 cooldown 时长的打开状态，期间所有操作毫秒级快速失败，
// 把 Redis 宕机从「每个请求阻塞 N 秒」收敛为「每个 cooldown 窗口仅一次探测性失败」，
// 其余请求直接回源 DB。cooldown 到期后自动恢复（半开：下一次真实访问决定是否继续熔断）。
//
// 重要：只有「读」操作会触发熔断（fail），「写」操作不会。这是刻意区分两种后端故障：
//   - 后端宕机/不可达：读与写都失败，读路径作为探针先触发熔断，随后读写一起快速失败；
//   - 后端内存满（maxmemory 达上限 + noeviction）：读仍然成功、只有写返回 OOM 错误。
//     若写失败也触发熔断，会把「写退化」错误地放大成「读也被切断」，反而让仍然可用的
//     读缓存被禁用、流量全部回源 DB。因此写失败只记日志 + 有界超时快速返回（软降级），
//     不打开熔断；熔断打开时写也会因 allow()==false 一并跳过。
type healthGate struct {
	mu        sync.Mutex
	cooldown  time.Duration
	failUntil time.Time
}

func newHealthGate(cooldown time.Duration) *healthGate {
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	return &healthGate{cooldown: cooldown}
}

// allow 报告当前是否允许访问后端。熔断打开期间返回 false。
func (g *healthGate) allow() bool {
	if g == nil {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Now().After(g.failUntil)
}

// fail 记录一次后端失败并打开熔断，持续 cooldown 时长。
func (g *healthGate) fail() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failUntil = time.Now().Add(g.cooldown)
}

// redisGate 为进程级共享「读」熔断器：gin-shared 只有一个 *redis.Client，后端宕机时
// 任何缓存类型（Hash/List/Provider）的读失败都能互相触发快速失败。
var redisGate = newHealthGate(10 * time.Second)

// redisWriteGate 是独立的「写」熔断器：写失败（典型为内存满 OOM）只熔断写、不熔断读，
// 避免 OOM 时写失败拖慢/切断仍然可用的读命中路径；同时避免每次未命中都各自阻塞 1s 去写。
var redisWriteGate = newHealthGate(10 * time.Second)
