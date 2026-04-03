# GinShared Package

The `ginshared` package provides shared utilities and components for building Gin-based web applications.

## Features

- **Gin Engine Setup**: Automatic initialization with logging and recovery
- **Dependency Injection**: Controller registration via Dig
- **Security Features**: CORS, GZIP, HTTPS enforcement
- **Monitoring**: Prometheus metrics, PProf profiling
- **WebSocket Support**: Built-in WebSocket handling
- **Request Locking**: Prevent duplicate requests
- **Static File Serving**: Configurable static file hosting

## Main Components

### Engine Initialization

- `Start()`: Starts the Gin server with all configured components
- `GetContainer()`: Returns the DI container for controllers

### Middleware

- **CORS**: Cross-origin resource sharing
- **GZIP**: Response compression
- **HTTPS**: Force HTTPS redirects
- **Security**: Security headers and protections
- **Iframe**: Clickjacking protection

### Utilities

- **Prometheus**: Metrics collection and exposure
- **PPProf**: Performance profiling endpoints
- **WebSocket**: WebSocket connection handling
- **RequestLocker**: Prevent concurrent duplicate requests

## Usage

```go
// Start the application
ginshared.Start()

// Register controllers via dependency injection
core.Provide(NewMyController)
```

## Configuration

- `address`: Server address (default: :5001)
- `shutdown`: Shutdown timeout (default: 3s)
- `baseUri`: Base API URI (default: /v1)
- `static.folder`: Static files directory
- `static.enabled`: Enable static file serving

## Dependencies

- Gin web framework
- Zap logging
- Viper configuration
- Prometheus metrics
