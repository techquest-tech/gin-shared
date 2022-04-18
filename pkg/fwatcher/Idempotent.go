package fwatcher

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type FileIdempotent struct {
	gorm.Model
	FileName string `gorm:"size:64"`
	FileKey  string `gorm:"size:64;unique"`
}

func init() {
	ginshared.GetContainer().Provide(func(logger *zap.Logger, db *gorm.DB) *FileIdempotentService {
		service := FileIdempotentService{
			Logger: logger.With(zap.String("service", "FileIdempotentService")),
			DB:     db,
			Key:    time.RFC3339,
		}

		settings := viper.Sub("fileWatch")

		if settings != nil {
			settings.Unmarshal(&service)
		}

		if viper.GetBool(ginshared.KeyInitDB) {
			db.AutoMigrate(&FileIdempotent{})
		}

		logger.Info("IdempotentService inited.")

		return &service
	})
}

type FileIdempotentService struct {
	Logger *zap.Logger
	Key    string
	DB     *gorm.DB
}

func (fs *FileIdempotentService) Idempotent(file string) (bool, error) {
	fileinfo, err := os.Stat(file)
	if err != nil {
		fs.Logger.Error("fetch file information failed.", zap.String("file", file), zap.Error(err))
		return true, err
	}
	fs.Logger.Debug("fetch file stat done", zap.Any("file", fileinfo))
	filekey := fmt.Sprintf("%s-%s", fileinfo.Name(), fileinfo.ModTime().Format(fs.Key))

	fi := FileIdempotent{}
	err = fs.DB.First(&fi, "file_key = ?", filekey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fs.Logger.Info("File key record not found", zap.String("FileKey", filekey))
			return false, nil
		}
		fs.Logger.Error("query FileKey records failed.", zap.Error(err))
		return false, err
	}
	if fi.ID != 0 {
		fs.Logger.Info("File key record found, file should be processed before.", zap.String("FileKey", filekey))
		return true, nil
	}
	fs.Logger.Warn("something wrong for file idempotent, should fi.ID should always > 0")
	return false, nil
}
