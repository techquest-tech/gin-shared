# Storage Package

The `storage` package provides unified filesystem abstraction with support for multiple storage backends.

## Features

- **Multi-Backend Support**: Local FS, OSS, SFTP
- **Afero Integration**: Built on Afero filesystem abstraction
- **Named Filesystems**: Register and retrieve filesystems by name
- **Caching**: Optional filesystem caching
- **Directory Management**: Create and manage directories

## Main Components

### Filesystem Service

- `GetFs(key)`: Get filesystem by key
- `CreateFs(key)`: Create filesystem with release function
- `EnsureDir(fs, dir)`: Create directory if not exists

### Storage Backends

- **Local**: Local filesystem access
- **OSS**: Alibaba Cloud Object Storage Service
- **SFTP**: SSH File Transfer Protocol

### OssSettings

Configuration for OSS:
- `Endpoint`: OSS endpoint URL
- `AccessKey`: Access key ID
- `SecretKey`: Access key secret
- `Bucket`: Bucket name
- `Region`: OSS region
- `Path`: Base path prefix

## Usage

```go
// Get filesystem
fs, release, err := storage.CreateFs("oss")
if err != nil {
    return err
}
defer release()

// Write file
err = afero.WriteFile(fs, "path/to/file.txt", data, 0644)

// Create directory
err = storage.EnsureDir(fs, "path/to/dir")
```

## Configuration

Configure storage backends via Viper:
```yaml
oss:
  endpoint: https://oss-cn-hangzhou.aliyuncs.com
  accessKey: your-access-key
  secretKey: your-secret-key
  bucket: my-bucket
  region: cn-hangzhou
  path: /app/data
```

## Dependencies

- Afero for filesystem abstraction
- FS-OSS for OSS integration
- Viper for configuration
- Zap for logging
