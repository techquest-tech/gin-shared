# Query Package

The `query` package provides flexible SQL query execution with dynamic WHERE conditions and paging support.

## Features

- **Raw SQL Queries**: Execute raw SQL with parameter binding
- **Dynamic WHERE**: Build WHERE conditions dynamically
- **Paging Support**: Automatic pagination with total count
- **Preset Parameters**: Support for default/fixed parameters
- **Configurable Queries**: Define queries in configuration

## Main Components

### RawQuery

Query definition structure:
- `Sql`: Raw SQL template
- `Params`: Parameter names in order
- `Where`: Map of WHERE conditions to parameter keys
- `Orderby`: ORDER BY clause
- `Groupby`: GROUP BY clause
- `Preset`: Default parameter values
- `SumEnabled`: Enable total count query

### Query Functions

- `Query[T](db, rawQuery, data)`: Execute query and return typed results
- `RawQuery.Query(db, data)`: Query returning map results
- `PagingResult()`: Get paginated results with totals

### PagingResult[T]

Generic pagination result:
- `Page`: Current page number
- `PageSize`: Items per page
- `Total`: Total record count
- `TotalPage`: Total pages
- `Data`: Result data

## Usage

```go
// Define query
query := &RawQuery{
    Sql: "SELECT * FROM users {{.where}}",
    Params: []string{"owner"},
    Where: map[string]string{
        "status": "status = ?",
        "role": "role LIKE ?",
    },
}

// Execute query
results, err := query.Query(db, map[string]any{
    "owner": "admin",
    "status": "active",
    "page": 0,
    "page_size": 100,
})
```

## Special Parameters

- `page`: Page number (0-indexed)
- `page_size`: Items per page (-1 for all)
- `orderby`: Dynamic ORDER BY
- `groupby`: Dynamic GROUP BY
- `limit`: Override limit
- `offset`: Override offset

## Dependencies

- GORM for database access
- Zap for logging
