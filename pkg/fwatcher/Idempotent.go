package fwatcher

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type FileIdempotentParams struct {
	Db *gorm.DB `optional:"true"`
}

type FileIdempotent struct {
	Logger *zap.Logger
	Key    string
	File   string
	DB     *gorm.DB
}
