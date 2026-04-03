# Cache Package

The `cache` package provides a generic caching system with support for different cache providers, including RAM, Redis, and GORM.

## Features

- **Generic Type Support**: Works with any type using Go generics
- **Multiple Providers**: RAM, Redis, and GORM cache providers
- **TTL Support**: Configurable time-to-live for cached items
- **Cached Results**: Helper for caching function results
- **Hash-based Keys**: Automatic key generation from request objects
- **List Operations**: Support for list-based caching (CachedList)
- **Hash Operations**: Support for hash-based caching (Hash)
- **GORM Integration**: Database-backed caching
- **Build Tag Support**: Use build tags to select cache implementation

## Main Components

### Core Interfaces

- **`CacheProvider[T]`**: Interface for cache providers
- **`Hash`**: Interface for hash operations (Redis, GORM)
- **`CachedList[T]`**: Interface for list operations
- **`WithKey`**: Interface for objects that provide their own key

### Cache Implementations

#### RAM Cache
- **`ram.go`**: In-memory cache with TTL support
- **`ramList.go`**: In-memory list cache
- **Default provider when no other provider is specified**
- **Build tag**: `ram`

#### Redis Cache
- **`redis.go`**: Redis-backed cache
- **`redisHash.go`**: Redis hash operations
- **`redisList.go`**: Redis list operations
- **Supports distributed caching across multiple instances**
- **Build tag**: `!ram` (default when RAM is not specified)

#### GORM Cache
- **`gormcache/`**: Database-backed cache using GORM
- **`gorm.go`**: GORM cache implementation
- **`gormHash.go`**: GORM hash operations
- **`gormList.go`**: GORM list operations
- **`models.go`**: Database models for cache storage
- **Persistent caching with database storage**
- **Build tag**: `gorm_cache`

### Utility Components

#### Cache[T]

Generic cache wrapper:
- `New[T]()`: Create cache with default timeout
- `NewWithTimeout[T](dur time.Duration)`: Create cache with custom timeout
- `Set(key string, value T)`: Store a value
- `Get(key string)`: Retrieve a value
- `Del(key string)`: Delete a cached value
- `Keys()`: Get all cache keys

#### CachedResult[T, R]

Caches function results based on request parameters:
- `NewCachedResult[T, R](timeout time.Duration)`: Create cached result handler
- `CacheFun1(ctx, req, fn)`: Execute function and cache result
- **Automatic key generation from request objects**

#### HashEx[T]

Extended hash operations with type support:
- `NewHashEx[T]()`: Create type-safe hash operations
- `GetValues(ctx, key, fields...)`: Get multiple values with type conversion
- `SetValues(ctx, key, values)`: Set multiple values with type handling
- `GetValue(ctx, key, field)`: Get single value with type conversion
- `GetAll(ctx, key)`: Get all hash fields with type conversion

## Hash and CachedList Usage

### Hash Operations

```go
// Get hash operations
hash := cache.NewHashEx[string]()

// Set multiple values
values := map[string]string{
    "name": "John",
    "email": "john@example.com",
    "status": "active",
}
err := hash.SetValues(ctx, "user:1", values)

// Get specific values
name, err := hash.GetValue(ctx, "user:1", "name")
email, err := hash.GetValue(ctx, "user:1", "email")

// Get all values
allValues, err := hash.GetAll(ctx, "user:1")

// Check if hash exists
exists, err := hash.Existed(ctx, "user:1")

// Set TTL
hash.SetTTL(ctx, "user:1", 24*time.Hour)
```

### CachedList Operations

```go
// Get list operations
list := cache.GetListCache[string]()

// Append items to list
list.Append(ctx, "users:recent", "user1", "user2", "user3")

// Get all items
users, err := list.GetAll(ctx, "users:recent")

// Delete list
list.Del(ctx, "users:recent")
```

## Build Tags for Cache Implementation

The cache package uses build tags to select different cache implementations. This allows you to choose the most appropriate cache provider for your environment without changing code.

### Available Build Tags

| Build Tag | Cache Provider | Description |
|-----------|----------------|-------------|
| `ram` | RAM Cache | In-memory cache, fastest but volatile |
| `!ram` (default) | Redis Cache | Distributed cache, requires Redis server |
| `gorm_cache` | GORM Cache | Database-backed cache, persistent |

### How to Use Build Tags

#### Using RAM Cache

```bash
# Build with RAM cache
go build -tags ram .

# Run with RAM cache
go run -tags ram .
```

#### Using Redis Cache (default)

```bash
# Build with Redis cache (default, no tag needed)
go build .

# Run with Redis cache
go run .
```

#### Using GORM Cache

```bash
# Build with GORM cache
go build -tags gorm_cache .

# Run with GORM cache
go run -tags gorm_cache .
```

### Important Note

When using `gorm_cache` tag, the system will:
- Use GORM as the primary cache provider
- Not use Redis or RAM cache
- Store cache data in the database
- Provide persistent caching

## Usage Examples

### Basic RAM Cache

```go
// Create cache with default timeout (30 minutes)
userCache := cache.New[*User]()

// Set value
userCache.Set("user:1", user)

// Get value
if user, found := userCache.Get("user:1"); found {
    // Use cached user
}

// Delete value
userCache.Del("user:1")
```

### Custom Timeout

```go
// Create cache with 1 hour timeout
productCache := cache.NewWithTimeout[*Product](1 * time.Hour)
```

### Cached Function Results

```go
// Create cached result handler
cachedResult := cache.NewCachedResult[[]Product, string](5 * time.Minute)

// Define function to cache
fetchProducts := func(ctx context.Context, category string) ([]Product, error) {
    // Database query or API call
    return db.Where("category = ?", category).Find(&[]Product{}).Error
}

// Use cached function
products, err := cachedResult.CacheFun1(ctx, "electronics", fetchProducts)
```

### Redis Cache

```go
// Redis cache is automatically used when Redis is available
// No code changes needed - the Cache interface remains the same

// The system will detect Redis configuration and use it as the backend
cache := cache.New[*User]() // Will use Redis if configured
```

### GORM Cache

```go
// GORM cache provides persistent storage
// Useful for long-term caching or when Redis is not available

// Build with gorm_cache tag
// go build -tags gorm_cache .

// The system will use GORM cache when configured
cache := cache.New[*Configuration]() // Will use GORM if built with gorm_cache tag
```

## Configuration

### Redis Configuration

Configure Redis via Viper:

```yaml
redis:
  host: localhost
  port: 6379
  password: ""
  db: 0
  poolSize: 10
  minIdleConns: 5
  dialTimeout: 5s
  readTimeout: 3s
  writeTimeout: 3s
  poolTimeout: 4s
```

### GORM Cache Configuration

GORM cache uses the same database configuration as the application:

```yaml
database:
  type: mysql
  connection: "user:pass@tcp(localhost:3306)/db"
  # other database settings...
```

### Cache Configuration

Configure specific cache behavior:

```yaml
cache:
  User:
    localItems: 1000  # Local cache items for Redis
    ttl: 1h          # Cache TTL
  Product:
    disabled: false   # Enable/disable cache
    localItems: 500
    ttl: 30m
```

## Performance Considerations

- **RAM Cache**: Fastest but volatile, limited by memory
- **Redis Cache**: Fast, distributed, but requires Redis server
- **GORM Cache**: Persistent, but slower than in-memory options

## Best Practices

1. **Choose the right provider** based on your needs:
   - Use RAM for ephemeral, frequently accessed data
   - Use Redis for distributed systems or larger datasets
   - Use GORM for persistent caching or when Redis is not available

2. **Set appropriate TTLs**:
   - Short TTLs (seconds/minutes) for frequently changing data
   - Longer TTLs (hours/days) for static data

3. **Use cached results** for expensive operations:
   - Database queries
   - API calls
   - Complex calculations

4. **Implement `WithKey` interface** for custom objects:
   ```go
   type User struct {
       ID   int
       Name string
   }

   func (u *User) Key() string {
       return fmt.Sprintf("user:%d", u.ID)
   }
   ```

5. **Use build tags** to select the appropriate cache implementation for your environment:
   - Development: `ram` tag for simplicity
   - Production: `gorm_cache` for persistence or default (Redis) for performance

## Dependencies

- **Zap**: For logging
- **Hashstructure**: For generating hash keys from objects
- **Redis**: For Redis cache provider (optional)
- **GORM**: For GORM cache provider (optional)
- **Go Cache**: For local in-memory caching with Redis

## Provider Selection

The cache system uses build tags to select the cache provider:

| Build Tag | Provider Selection |
|-----------|-------------------|
| `gorm_cache` | GORM Cache (highest priority) |
| `ram` | RAM Cache |
| `!ram` (default) | Redis Cache |

## Advanced Usage

### Custom Cache Provider

```go
// Implement CacheProvider interface
type MyCustomCache[T any] struct {
    // Implementation
}

func (c *MyCustomCache[T]) Set(key string, value T) { /* ... */ }
func (c *MyCustomCache[T]) Get(key string) (T, bool) { /* ... */ }
func (c *MyCustomCache[T]) Del(key string) error { /* ... */ }
func (c *MyCustomCache[T]) Keys() []string { /* ... */ }

// Use custom provider
cache := &cache.Cache[T]{
    cc: &MyCustomCache[T]{},
}
```

### Hash Operations with Custom Types

```go
// Hash operations with structs
type UserProfile struct {
    Name     string
    Email    string
    Age      int
    LastSeen time.Time
}

hash := cache.NewHashEx[UserProfile]()

// Set profile
profile := UserProfile{
    Name:     "John Doe",
    Email:    "john@example.com",
    Age:      30,
    LastSeen: time.Now(),
}

// Store as JSON in hash
err := hash.SetValues(ctx, "user:1:profile", map[string]UserProfile{
    "profile": profile,
})

// Retrieve with type conversion
storedProfile, err := hash.GetValue(ctx, "user:1:profile", "profile")
```

### List Operations with Complex Types

```go
// List operations with structs
type LogEntry struct {
    Timestamp time.Time
    Level     string
    Message   string
}

list := cache.GetListCache[LogEntry]()

// Add log entries
logs := []LogEntry{
    {time.Now(), "INFO", "Application started"},
    {time.Now(), "DEBUG", "Processing request"},
    {time.Now(), "INFO", "Request completed"},
}

for _, log := range logs {
    list.Append(ctx, "app:logs", log)
}

// Retrieve all logs
storedLogs, err := list.GetAll(ctx, "app:logs")
```
