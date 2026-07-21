# Storage Package

The `storage` package provides a unified filesystem abstraction based on `afero.Fs`, with support for local filesystem, OSS, and SFTP backends.

## Features

- **Multi-Backend Support**: Local FS, OSS, SFTP
- **Afero Integration**: Unified read/write access through `afero.Fs`
- **Per-Key Factory Model**: Build filesystem instances and public URL functions by storage key
- **Public URL Support**: Generate public URLs for OSS or resource URLs for FS-based backends
- **Directory Management**: Ensure directory existence through `EnsureDir`

## Main APIs

### Filesystem

- `CreateFs(key)`: Create a filesystem instance and its `release` function
- `EnsureDir(fs, dir)`: Create directory if it does not exist

### Public URL

- `GetPublicURLFunc(key)`: Get a `PublicURLFunc` bound to the given storage key
- `GetPublicURLFuncDefault()`: Get a `PublicURLFunc` bound to default key `fileroot`
- `CreatePublicURL(key, fullFileName)`: Generate a public URL with an explicit key
- `CreatePublicURLDefault(fullFileName)`: Generate a public URL with default key `fileroot`
- `CreatePublicURLWithFunc(fn, fullFileName)`: Generate a public URL with a previously resolved function

### Backends

- **Local / SFTP**: Return a resource URL such as `/v1/resources/{xid}` and serve file content through the built-in Gin route
- **OSS**: Return the direct public object URL based on `bucket + endpoint + fullFileName`

## Public Resource Route

For FS-based backends, the package registers a public resource route automatically:

- Route: `/v1/resources/:xid`
- Behavior: resolve `xid` to the original file path and stream file content through `afero.Fs`
- Expiration: `xid` is cached for `24h`
- Failure: expired `xid` or missing file returns `404`

If `domain` is configured, generated resource URLs will include the full domain; otherwise they will return a relative URI.

## Default Key

Unless otherwise specified, the package uses the default storage key:

```go
const defaultStorageKey = "fileroot"
```

If `key` is empty or not configured in Viper, APIs such as `CreateFs` and `GetPublicURLFunc` will fall back to `fileroot`.

## Usage

### Create Filesystem

```go
fs, release, err := storage.CreateFs("fileroot")
if err != nil {
	return err
}
defer release()

if err := storage.EnsureDir(fs, "owner-a/blockedEpc/202607"); err != nil {
	return err
}

if err := afero.WriteFile(fs, "owner-a/blockedEpc/202607/epc.png", data, 0o644); err != nil {
	return err
}
```

### Create Public URL With Explicit Key

```go
publicURL, err := storage.CreatePublicURL("fileroot", "/data/upload/owner-a/blockedEpc/202607/epc.png")
if err != nil {
	return err
}
```

### Create Public URL With Default Key

```go
publicURL, err := storage.CreatePublicURLDefault("/data/upload/owner-a/blockedEpc/202607/epc.png")
if err != nil {
	return err
}
```

### Reuse PublicURLFunc

```go
fn, err := storage.GetPublicURLFunc("fileroot")
if err != nil {
	return err
}

publicURL, err := storage.CreatePublicURLWithFunc(fn, "/data/upload/owner-a/blockedEpc/202607/epc.png")
if err != nil {
	return err
}
```

## Configuration

Configure storage backends via Viper.

### Local

```yaml
fileroot:
  type: local
  path: /data/upload
```

### OSS

```yaml
fileroot:
  type: oss
  endpoint: https://oss-cn-hangzhou.aliyuncs.com
  accessKey: your-access-key
  secretKey: your-secret-key
  bucket: my-bucket
  region: cn-hangzhou
  path: upload
```

### SFTP

```yaml
fileroot:
  type: sftp
  endpoint: sftp.example.com:22
  account: demo
  password: your-password
  path: /data/upload
```

### Optional Base URL

```yaml
baseUri: /v1
domain: https://example.com
```

## Notes

- The package no longer keeps a global filesystem cache; `CreateFs` returns a fresh instance on each call
- For OSS, public URL generation is direct and does not go through `/v1/resources/:xid`
- For FS-based backends, `fullFileName` should match the actual file path that can be resolved under the configured root

## Dependencies

- Afero for filesystem abstraction
- FS-OSS for OSS integration
- Viper for configuration
- Zap for logging
- Gin for public resource routing
