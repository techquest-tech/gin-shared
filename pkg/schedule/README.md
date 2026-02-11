# Schedule Module

This module provides a robust cron job scheduling mechanism with support for both distributed environments (via Redis) and single-instance deployments (RAM mode).

## Features

### 1. Distributed Scheduling (Default)
Designed for high availability in Kubernetes/Cluster environments. It ensures that tasks are executed exactly once across the cluster by restricting execution to a single "Leader" Pod.

*   **Mechanism**:
    *   **Leader Election**: All Pods compete to become the leader using a Redis Lock (`SetNX` with TTL). The election Key is scoped by `core.AppName` to support multi-tenant Redis sharing.
    *   **Leader-Only Execution**: Only the elected Leader Pod will trigger and execute the cron tasks. Non-leader Pods remain in standby.
    *   **Failover**: The Leader periodically renews its lease. If the Leader Pod crashes, the lease expires (default TTL 10s), and a standby Pod is automatically elected as the new Leader to resume operations.
*   **Dependencies**: Requires Redis.

### 2. RAM Mode (Simple)
Designed for development or single-instance deployments where Redis is not available or needed.

*   **Mechanism**: Standard in-memory Cron.
*   **Limitations**: No concurrency control (NoLocker). If you run multiple replicas, the task will run on *every* replica.
*   **Dependencies**: None (In-memory).

## Build Tags

The module uses Golang Build Tags to switch implementation details at compile time, similar to `pkg/cache`.

| Mode | Build Tag | Features | Concurrent Control |
| :--- | :--- | :--- | :--- |
| **Default** | `!ram` | Leader Election, Redis Lock, Job History | Yes (Distributed Leader) |
| **RAM** | `ram` | Local Cron | No (None) |

## Usage

### Enable Distributed Mode (Default)
Simply build your project normally. The code defaults to using `cron_redis.go`.

```bash
go build ./...
```

**Configuration**:
The module automatically registers a startup hook (`core.ProvideStartup`) to start the leader election process when the application starts. No manual invocation is needed if you use `ginshared.Start()`.

You can configure the Leader Election parameters via your application configuration (e.g., `app.yaml`):

```yaml
schedule:
  leader:
    interval: 3s       # Election check interval (default: 3s)
    ttl: 10s           # Leader key validity period (default: 10s)
    key: "my-leader"   # Optional: Override redis key (default: scheduler:<AppName>:leader)
```

**Code Example**:

```go
import "github.com/techquest-tech/gin-shared/pkg/schedule"

func main() {
    // ... init redis ...
    
    // Define tasks
    schedule.CreateScheduledJob("my-task", "*/1 * * * *", myFunc)
    
    // Start application (Leader Election starts automatically)
    ginshared.Start()
}
```

### Enable RAM Mode
Use the `ram` build tag. This will compile `cron_ram.go` and exclude Redis dependencies.

```bash
go build -tags ram ./...
```
