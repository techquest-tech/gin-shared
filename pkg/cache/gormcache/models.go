package gormcache

import (
	"time"

	"gorm.io/gorm"
)

type CacheEntry struct {
	ID        uint           `gorm:"primaryKey"`
	Key       string         `gorm:"size:255;uniqueIndex;not null"`
	Value     string         `gorm:"type:text"`
	ExpiresAt *time.Time     `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type HashEntry struct {
	ID        uint           `gorm:"primaryKey"`
	Key       string         `gorm:"size:255;index;not null"`
	Field     string         `gorm:"size:255;not null"`
	Value     string         `gorm:"type:text"`
	ExpiresAt *time.Time     `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type ListEntry struct {
	ID        uint           `gorm:"primaryKey"`
	Key       string         `gorm:"size:255;index;not null"`
	Value     string         `gorm:"type:text"`
	SortOrder int            `gorm:"index"`
	ExpiresAt *time.Time     `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
