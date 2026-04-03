# ORM Package

The `orm` package provides database abstraction and utilities built on top of GORM.

## Features

- **Multi-Database Support**: MySQL, PostgreSQL, SQLite, SQL Server, ODBC
- **Auto Migration**: Automatic table and view creation
- **Database Health Check**: Health monitoring endpoints
- **Query Utilities**: Paging, common query patterns
- **Database Logging**: Integrated GORM logging with Zap
- **Entity Registration**: Dynamic entity registration for migration

## Main Components

### Database Initialization

- `InitDefaultDB()`: Initialize default database connection
- `InitDB(sub, logger)`: Initialize named database connection
- `InitDBWithPrefix(sub, prefix)`: Initialize with table prefix

### Connection Management

- `DialectorMap`: Map of database driver dialects
- `Connections`: Map of active database connections
- Support for multiple database connections

### Utilities

- `QueryBase`: Base struct for paging queries
- `PagingResult[T]`: Generic paging result with total count
- `AppendEntity()`: Register entities for auto-migration
- `MigrateTableAndView()`: Migrate tables and database views

### Database Drivers

- MySQL (primary)
- PostgreSQL
- SQLite
- SQL Server
- ODBC (legacy)

## Configuration

Configure via Viper:
- `type`: Database type (mysql, postgres, etc.)
- `connection`: Connection string
- `maxLifetime`: Connection max lifetime
- `max`: Max open connections
- `idel`: Max idle connections
- `tablePrefix`: Table name prefix
- `initDB`: Enable auto migration

## Usage

```go
// Register entity for migration
orm.AppendEntity(&MyModel{})

// Use default DB connection
db := core.GetService[*gorm.DB]()

// Query with paging
var result orm.PagingResult[MyModel]
result.Trigger(db, queryBase)
```

## Dependencies

- GORM
- Zap logging
- Viper configuration
- GinShared for integration
