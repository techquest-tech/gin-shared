# Messaging Package

The `messaging` package provides a publish-subscribe messaging system for asynchronous communication.

## Features

- **Pub/Sub Pattern**: Publish messages to topics, subscribe with consumers
- **Redis Streaming**: Default implementation using Redis streams
- **GORM Integration**: Sync service for database message persistence
- **Message Processing**: Pluggable processor functions

## Main Components

### MessagingService

Core messaging interface:
- `Pub(ctx, topic, payload)`: Publish message to topic
- `Sub(ctx, topic, consumer, processor)`: Subscribe to topic with consumer group

### Processor

Function type for message processing:
- Receives topic, consumer, and payload
- Returns error on processing failure

### Implementations

- **Redis Streaming**: High-performance Redis-based messaging
- **RAM**: In-memory messaging for testing
- **GORM Sync Service**: Database-backed message synchronization

## Usage

```go
// Publish message
err := msgService.Pub(ctx, "orders", orderData)

// Subscribe
err := msgService.Sub(ctx, "orders", "consumer-1", func(ctx, topic, consumer, payload) {
    // Process message
})
```

## 发送端内存看门狗（streamguard.go）

`XADD ... MAXLEN` 是**按条数**、且带 `denyoom` 的原子命令：实例 `maxmemory` 打满时，写入与裁剪
会**一起失效**（2026-10-12 monitor.tracing 打满 1 GiB 实例，上游静默 111 分钟）。因此 `Pub` 在写入
成功后调用 `MaybeGuard`，按**字节**预算兜底裁剪。**缺省关闭**，只在显式配置的服务启用。

### 配置 sample（放在服务配置的 `messaging:` 下）

```yaml
messaging:
  settings:
    'your.business.stream': 100
  guard:
    enabled: true # 缺省 false；只有需要兜底的服务才开
    topics: # 白名单，只填监控/可裁剪流；缺省为空 = 一条都不裁
      - monitor.tracing
      - monitor.schedule
      - monitor.error
    maxBytes: 157286400 # 单流字节预算（150 MiB）。必须写数字，不能写 "150MiB"
    keepHours: 6 # 第一步：XTRIM MINID 保留最近 6 小时（0 = 跳过这一步）
    minEntries: 100 # 第二步：按条数减半收敛的下限（保命优先，不再往下裁）
    minInterval: 60s # 每个进程最快的检查间隔（节流）；Pub 热路径几乎零开销
    alertInterval: 1h # 同一条流两次上报 monitor.error 的最小间隔
    lock: # 多节点互斥：每轮只让一个节点真正裁剪
      enabled: true
      ttl: 90s # 租约（应 > minInterval）；持有者挂掉后自动释放
      keyPrefix: messaging:guard:lock # 实际键 "<keyPrefix>:<topic>"
```

其它服务**什么都不用配**（缺省 `enabled: false`，不会裁剪任何流）。

### 行为要点

| 项 | 说明 |
| --- | --- |
| 触发时机 | 写入成功后判断：不在白名单 / 未到 `minInterval` 窗口 → 直接返回；命中则起 goroutine 执行，不占请求延迟 |
| 内存口径 | `MEMORY USAGE <topic> SAMPLES 0`（遍历全部节点）。默认采样只抽 5 个节点外推，对条目大小不均的流会低估（实测差 28 倍），低估即不触发 |
| 裁剪顺序 | 先 `XTRIM MINID`（按时间）→ 再按条数减半收敛到 `minEntries`（最多 16 轮），每轮复测直到进预算 |
| 多节点 | `SET NX PX` 抢锁，每轮只有一个节点裁；锁命令本身是 `denyoom`，**OOM 时会失败 → fail-open 照常裁剪**（宁可重复也不能不裁） |
| 告警 | 限频上报 `monitor.error`，`Uri = redis.streamguard/<topic>`；反复触发说明单条 payload 过大，需生产端做 Body/Resp 外置或截断 |
| 日志 | 裁剪时 `streamguard trimmed stream`（warn）；抢锁失败跳过为 debug；抢锁报错为 warn |

### 生效确认

1. 启动/首条消息后，Redis 出现锁键：`GET messaging:guard:lock:monitor.tracing`（TTL ≤ `lock.ttl`）；
2. 裁剪发生时日志出现 `streamguard trimmed stream`，字段含 `bytesBefore/bytesAfter/budget/entriesBefore/entriesAfter/rounds`；
3. 若日志出现 `streamguard lock failed, proceed anyway (fail-open)`，说明实例当时已打满（锁命令被拒），此时裁剪仍在继续——这正是设计目标。

## Dependencies

- Redis for streaming implementation
- GORM for database sync
- Zap for logging
