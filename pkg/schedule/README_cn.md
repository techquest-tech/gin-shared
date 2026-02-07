# Schedule 模块

本模块提供了一个健壮的定时任务调度机制，支持分布式环境（通过 Redis）和单实例部署（RAM 模式）。

## 功能特性

### 1. 分布式调度（默认）
专为 Kubernetes/集群环境的高可用性设计。它通过限制仅由单个“Leader” Pod 执行任务，确保任务在整个集群中精确执行一次。

*   **机制**：
    *   **Leader 选举 (Leader Election)**：所有的 Pods 通过 Redis 锁（`SetNX` + TTL）竞争成为 Leader。选举 Key 包含 `core.AppName` 以支持多租户共享 Redis。
    *   **仅 Leader 执行 (Leader-Only Execution)**：只有当选的 Leader Pod 会触发并执行定时任务。非 Leader Pods 保持待命状态。
    *   **故障转移 (Failover)**：Leader 会定期续租。如果 Leader Pod 崩溃，租约过期（TTL 10秒）后，备用 Pod 会自动被选举为新 Leader 并恢复运行。
*   **依赖**：需要 Redis。

### 2. RAM 模式（简单）
专为开发环境或不需要 Redis 的单实例部署设计。

*   **机制**：标准的内存 Cron。
*   **限制**：无并发控制（NoLocker）。如果你运行多个副本，任务将在*每个*副本上运行。
*   **依赖**：无（纯内存）。

## 构建标签 (Build Tags)

本模块使用 Golang Build Tags 在编译时切换实现细节，类似于 `pkg/cache`。

| 模式 | Build Tag | 特性 | 并发控制 |
| :--- | :--- | :--- | :--- |
| **Default** | `!ram` | Leader 选举, Redis 锁, 任务历史 | 有 (分布式 Leader) |
| **RAM** | `ram` | 本地 Cron | 无 |

## 使用方法

### 启用分布式模式（默认）
正常构建项目即可。代码默认使用 `cron_redis.go`。

```bash
go build ./...
```

**配置**：
本模块会自动注册启动钩子 (`core.ProvideStartup`)，在应用启动时开启 Leader 选举流程。如果你使用 `ginshared.Start()`，则无需手动调用。

**代码示例**：

```go
import "github.com/techquest-tech/gin-shared/pkg/schedule"

func main() {
    // ... init redis ...
    
    // 定义任务
    schedule.CreateScheduledJob("my-task", "*/1 * * * *", myFunc)
    
    // 启动应用 (Leader 选举会自动开始)
    ginshared.Start()
}
```

### 启用 RAM 模式
使用 `ram` build tag。这将编译 `cron_ram.go` 并排除 Redis 依赖。

```bash
go build -tags ram ./...
```
