# MQTT Client Package

The `mqttclient` package provides MQTT (Message Queuing Telemetry Transport) client functionality for IoT and messaging applications.

## Features

- **MQTT Protocol Support**: Connect to MQTT brokers
- **Message Handling**: Subscribe and publish to MQTT topics
- **Utility Functions**: Helper functions for MQTT operations

## Main Components

### MQTT Client

Wraps the Paho MQTT client for:
- Connecting to MQTT brokers
- Subscribing to topics
- Publishing messages
- Handling disconnections

### Utilities

Helper functions for:
- Message parsing
- Topic management
- Connection handling

## Usage

```go
// Connect to MQTT broker
client := mqttclient.New(config)

// Subscribe to topic
client.Subscribe("sensors/temperature", func(msg mqtt.Message) {
    // Handle message
})

// Publish message
client.Publish("sensors/humidity", payload)
```

## Configuration

- Broker URL
- Client ID
- Authentication credentials
- QoS levels
- Connection options

## Dependencies

- Eclipse Paho MQTT Go client
- Zap for logging
