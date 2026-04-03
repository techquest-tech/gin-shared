# Core Package

The `core` package provides fundamental infrastructure for dependency injection and application lifecycle management.

## Features

- **Dependency Injection**: Built on Uber's Dig library
- **Service Registration**: Easy service provisioning and injection
- **Startup Hooks**: Register startup callbacks for services
- **Event Bus Integration**: Publish-subscribe event system
- **Configuration Management**: AES encryption for sensitive config
- **Logging**: Structured logging with file appender
- **IDempotency**: Support for idempotent operations

## Main Components

### Container

Global Dig container for dependency injection:
- `Provide()`: Register constructors
- `ProvideStartup()`: Register startup callbacks
- `GetService[T]()`: Retrieve service instances

### EventBus

Event publishing and subscribing:
- Publish events across the application
- Subscribe to system events (e.g., DB initialized)

### Utilities

- **AES**: Configuration encryption/decryption
- **BlockingQueue**: Thread-safe queue implementation
- **FileService**: File system operations
- **FileAppender**: Log file rotation and management
- **IDempotent**: Idempotency key handling

## Usage

```go
// Register a service
core.Provide(NewMyService)

// Register startup callback
core.ProvideStartup(func(s *MyService) core.Startup {
    // Initialize service
    return nil
})

// Get service instance
svc := core.GetService[*MyService]()
```

## Dependencies

- Uber Dig for dependency injection
- Zap for logging
- Viper for configuration
