# Parquet Package

The `parquet` package provides Parquet file writing capabilities for efficient data storage and analytics.

## Features

- **Parquet File Writing**: Write data to Parquet format
- **Buffered Writing**: Batch messages for efficient I/O
- **Schema Validation**: Automatic schema-based sanitization
- **Compression Support**: GZIP and other compression codecs
- **Storage Integration**: Works with local, OSS, SFTP storage
- **UTF-8 Sanitization**: Automatic invalid UTF-8 handling

## Main Components

### ParquetDataService

Main service for writing Parquet files:
- `WriteMessages(msgs)`: Write messages to Parquet file
- `Start(ctx)`: Start continuous processing loop
- Configurable buffer size and duration
- Automatic file rotation

### ParquetSetting

Configuration for Parquet writing:
- `FsKey`: Storage filesystem key
- `Folder`: Output directory
- `FilenamePattern`: File naming pattern
- `BufferSize`: Batch size for buffering
- `BufferDur`: Time-based flush interval
- `Compress`: Compression codec (e.g., GZIP)
- `Ackfile`: Generate acknowledgment files

### Schema Handling

- Automatic schema inference from Go types
- Message sanitization by schema
- UTF-8 validation and correction

## Usage

```go
// Create service with settings
settings := &ParquetSetting{
    Folder: "data",
    BufferSize: 10000,
    BufferDur: 30 * time.Minute,
    Compress: "GZIP",
}

service := parquet.NewParquetDataServiceT[MyType](settings, "chunk_%s.parquet", channel)

// Start processing
go service.Start(ctx)
```

## Dependencies

- Parquet-go library
- Storage package for filesystem abstraction
- Messaging package for error handling
- Zap for logging
