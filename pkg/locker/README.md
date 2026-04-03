# Locker Package

The `locker` package provides distributed locking mechanisms for concurrent resource access control.

## Features

- **Distributed Locking**: Prevent concurrent access to shared resources
- **Multiple Implementations**: Local and Redis-based lockers
- **Timeout Support**: Lock with timeout to prevent deadlocks
- **Wait for Locker**: Block until lock is available

## Main Components

### Locker Interface

Core locking interface:
- `Lock(ctx, resource)`: Acquire a lock immediately
- `LockWithTimeout(ctx, resource, timeout)`: Acquire lock with timeout
- `WaitForLocker(ctx, resource, maxWait, timeout)`: Wait for lock availability

### Release

Function type for releasing locks:
- Returns error if release fails
- Should be called in defer statement

## Implementations

- **Local**: In-memory locking for single-instance applications
- **Redis**: Distributed locking for multi-instance deployments

## Usage

```go
// Acquire lock
release, err := locker.Lock(ctx, "resource-key")
if err != nil {
    return err
}
defer release(ctx)

// Critical section
// ...
```

## Dependencies

- Context for cancellation
- Redis (for distributed locking)
