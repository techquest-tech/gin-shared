# Dedup Package

The `dedup` package provides object deduplication and fingerprinting functionality using MD5 hashing.

## Features

- **Object Fingerprinting**: Generate unique MD5 fingerprints for objects
- **Deduplication**: Track and detect duplicate objects
- **Database Persistence**: Store fingerprints in database with GORM
- **Flexible Hashing**: Support custom hash providers or JSON serialization

## Main Components

### ObjectFingerprint

Database model for storing object fingerprints:
- `ObjectKey`: Unique identifier for object category
- `ObjectName`: Specific object name
- `MD5`: MD5 hash fingerprint
- Composite unique index on (ObjectKey, ObjectName)

### ObjectFingerprintService

Service for managing object fingerprints:
- `Set(key, name, obj)`: Store or update object fingerprint
- `Get(key, name)`: Retrieve stored fingerprint
- `IsDuplicated(key, name, obj)`: Check if object is duplicate

### HashBytesProvider

Interface for custom hashing:
- Implement `HashBytes()` to provide custom byte representation
- Falls back to JSON marshaling if not implemented

### BuildObjectMD5

Utility function to generate MD5 hash:
- Accepts any object type
- Uses custom hasher or JSON serialization
- Returns hex-encoded MD5 string

## Usage

```go
// Store object fingerprint
fingerprint, err := ServiceObjectFingerprint.Set("category", "item1", myObject)

// Check for duplicates
isDup, err := ServiceObjectFingerprint.IsDuplicated("category", "item1", newObj)
if isDup {
    // Object is duplicate
}
```

## Dependencies

- GORM for database operations
- Core package for dependency injection
- Crypto/md5 for hashing
