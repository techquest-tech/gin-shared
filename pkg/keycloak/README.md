# Keycloak Package

The `keycloak` package provides Keycloak integration for authentication and authorization in Gin applications.

## Features

- **Keycloak Authentication**: Seamless integration with Keycloak IAM
- **Role-based Access Control**: Restrict access by realm roles
- **Token Validation**: Automatic JWT token validation
- **Debug Mode**: Optional token logging for debugging

## Main Components

### KeycloakConfig

Configuration and access control builder:
- `Auth(roles...)`: Create role-based access middleware
- Configurable default roles

### Middleware

- `MustLogin()`: Enforces login requirement, validates Keycloak tokens
- `Auth(roles...)`: Role-based access control middleware

## Configuration

Configure via Viper under `keycloak` key:
- `url`: Keycloak server URL
- `realm`: Realm name
- `client-id`: Client ID
- `roles`: Default required roles

## Usage

```go
// Enforce login
router.GET("/protected", keycloak.MustLogin(), handler)

// Role-based access
router.GET("/admin", kc.Auth("admin"), adminHandler)
```

## Dependencies

- gin-keycloak library
- Viper for configuration
- Zap for logging
