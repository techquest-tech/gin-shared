# Types Package

The `types` package provides custom type definitions and utilities for common data types.

## Features

- **DateTime Type**: Custom time type with JSON serialization
- **Standard Format**: Consistent datetime formatting across the application
- **JSON Integration**: Automatic marshaling/unmarshaling

## Main Components

### DateTime

Custom time type for consistent datetime handling:

- Based on `time.Time`
- Format: `2006-01-02 15:04:05` (local time)
- JSON marshaling/unmarshaling support

### Methods

- `UnmarshalJSON(data)`: Parse from JSON string
- `MarshalJSON()`: Convert to JSON string
- `String()`: Format as string
- `Time()`: Convert to standard time.Time

## Usage

```go
// Use DateTime in structs
type MyModel struct {
    CreatedAt types.DateTime
}

// Automatically handles JSON serialization
// Input: "2024-01-15 10:30:00"
// Output: "2024-01-15 10:30:00"
```

## Format

The datetime format is: `2006-01-02 15:04:05`

This follows Go's reference time format and represents:
- Year-Month-Day Hour:Minute:Second
- Local timezone

## Dependencies

- Standard library time package
