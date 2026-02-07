# Schedule Module

This module provides a robust cron job scheduling mechanism with support for both distributed environments (via Redis) and single-instance deployments (RAM mode).

## Features

### 1. Distributed Scheduling (Default)
Designed for high availability in Kubernetes/Cluster environments. It solves the problem where a specific Pod failure could lead to missed tasks.

*   **Mechanism**:
    *   **Decoupled Trigger & Execution**: The scheduling (Cron) and execution are decoupled using **Redis Stream**.
    *   **Trigger Phase**: All Pods attempt to trigger the task. A `SetNX` lock ensures only one "Trigger Event" is produced per schedule time (deduplicated by minute).
    *   **Execution Phase**: A Consumer Group worker pool (running on all Pods) listens to the stream. Only one healthy Pod will claim and execute the task.
    *   **Failover**: If the executing Pod crashes, the message remains "Pending". Other healthy Pods will detect and reclaim (Claim) the message after a timeout, ensuring the task is eventually executed.
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
| **Default** | `!ram` | Redis Stream, Redis Lock, Job History | Yes (Distributed) |
| **RAM** | `ram` | Local Cron | No (None) |

## Usage

### Enable Distributed Mode (Default)
Simply build your project normally. The code defaults to using `cron_redis.go`.

```bash
go build ./...
```

**Configuration**:
The module automatically registers a startup hook (`core.ProvideStartup`) to launch the stream worker when the application starts. No manual invocation is needed if you use `ginshared.Start()`.

If you are not using `ginshared.Start()`, you can manually start the worker:

```go
import "github.com/techquest-tech/gin-shared/pkg/schedule"

func main() {
    // ... init redis ...
    
    // Define tasks
    schedule.CreateSchedule("my-task", "*/1 * * * *", myFunc)
    
    // Manually start if not using standard bootstrap
    schedule.StartStreamWorker()
}
```

### Enable RAM Mode
Use the `ram` build tag. This will compile `cron_ram.go` and exclude Redis dependencies.

```bash
go build -tags ram ./...
```
