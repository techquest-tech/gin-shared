# gin-shared

A comprehensive Go module that integrates Gin, Viper, Uber Dig (dependency injection), Zap logging, and Prometheus monitoring.

## Overview

This module provides a collection of reusable packages for building production-ready Go web applications with best practices built-in.

## Package Structure

### Core Infrastructure

- **[pkg/core](pkg/core/README.md)** - Dependency injection, event bus, configuration management, and application lifecycle
- **[pkg/ginshared](pkg/ginshared/README.md)** - Gin web framework utilities, middleware, and server initialization

### Authentication & Security

- **[pkg/auth](pkg/auth/README.md)** - API key authentication and user management
- **[pkg/keycloak](pkg/keycloak/README.md)** - Keycloak IAM integration for OAuth2/OIDC

### Data & Storage

- **[pkg/orm](pkg/orm/README.md)** - Database abstraction with GORM (MySQL, PostgreSQL, SQLite, SQL Server)
- **[pkg/dedup](pkg/dedup/README.md)** - Object deduplication and MD5 fingerprinting
- **[pkg/query](pkg/query/README.md)** - Flexible SQL query execution with dynamic WHERE and paging
- **[pkg/cache](pkg/cache/README.md)** - Generic caching system with RAM and Redis providers
- **[pkg/storage](pkg/storage/README.md)** - Unified filesystem abstraction (Local, OSS, SFTP)

### Messaging & Communication

- **[pkg/messaging](pkg/messaging/README.md)** - Publish-subscribe messaging with Redis streaming
- **[pkg/mqttclient](pkg/mqttclient/README.md)** - MQTT client for IoT messaging
- **[pkg/locker](pkg/locker/README.md)** - Distributed locking (local and Redis-based)

### Data Processing

- **[pkg/parquet](pkg/parquet/README.md)** - Parquet file writing for efficient data storage

### Scheduling

- **[pkg/schedule](pkg/schedule/README.md)** - Cron job scheduling with database persistence

### Notifications

- **[pkg/notify](pkg/notify/README.md)** - Email notification service (SMTP)

### Utilities

- **[pkg/types](pkg/types/README.md)** - Custom type definitions (DateTime with JSON support)

## Key Features

- **Dependency Injection**: Built on Uber's Dig for clean architecture
- **Multi-Database Support**: MySQL, PostgreSQL, SQLite, SQL Server
- **Caching**: RAM and Redis cache providers
- **Monitoring**: Prometheus metrics integration
- **Logging**: Structured logging with Zap
- **Configuration**: Viper-based configuration with encryption
- **Security**: API key auth, Keycloak integration, CORS, HTTPS
- **Messaging**: Redis streaming, MQTT support
- **Storage**: Local, OSS, SFTP filesystem abstraction

## Quick Start

```go
package main

import (
    "github.com/techquest-tech/gin-shared/pkg/core"
    "github.com/techquest-tech/gin-shared/pkg/ginshared"
    "github.com/techquest-tech/gin-shared/pkg/orm"
)

func main() {
    // Register your services
    core.Provide(NewMyService)
    
    // Start the application
    ginshared.Start()
}
```

## Configuration

Configure via Viper (config.yaml, ENV, etc.):

```yaml
database:
  type: mysql
  connection: "user:pass@tcp(localhost:3306)/db"
  maxLifetime: 1h
  max: 100
  idel: 10
  initDB: true

address: :5001
baseUri: /v1
```

## Dependencies

- [Gin](https://github.com/gin-gonic/gin) - Web framework
- [Viper](https://github.com/spf13/viper) - Configuration
- [Uber Dig](https://github.com/uber-go/dig) - Dependency injection
- [Zap](https://github.com/uber-go/zap) - Logging
- [GORM](https://github.com/go-gorm/gorm) - ORM library
- [Prometheus](https://github.com/prometheus/client_golang) - Metrics

## License

MIT License
