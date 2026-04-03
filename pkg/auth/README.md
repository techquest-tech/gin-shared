# Auth Package

The `auth` package provides authentication and authorization functionality for Gin-based applications.

## Features

- **API Key Authentication**: Validates API keys from query parameters, POST forms, or headers
- **User Management**: Create and list authenticated users with roles and permissions
- **Owner-based Access Control**: Supports multi-tenant access control with owner isolation
- **Store User Mapping**: Maps users to specific store codes for granular access
- **Key Expiration & Suspension**: Supports API key expiration dates and suspension

## Main Components

### AuthService

Core service for API key validation and user management:
- `Validate(key string)`: Validates an API key and returns user info
- `CreateUser()`: Creates new API keys for users
- `ListUsers()`: Lists users by owner

### AuthKey Model

Database model for storing API keys with:
- Username, owner, role, and remark
- Expiration and suspension status
- Unique API key (SHA256 hashed)

### Middleware

- `Auth()`: Gin middleware for API key authentication
- `NewAuthedRouter()`: Creates authenticated router groups with error handling

## Usage

```go
// Authentication middleware will validate API keys
authed := router.Group("/api").Use(auth.Auth)
```

## Dependencies

- GORM for database operations
- Zap for logging
- Viper for configuration
