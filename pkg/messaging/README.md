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

## Dependencies

- Redis for streaming implementation
- GORM for database sync
- Zap for logging
