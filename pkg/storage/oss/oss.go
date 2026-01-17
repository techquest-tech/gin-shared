package oss

import (
	"errors"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/techquest-tech/fsoss"
	"github.com/techquest-tech/gin-shared/pkg/storage"
	"go.uber.org/zap"
)

type OssSettings struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	Path      string
}

func init() {
	storage.NamedFsService["oss"] = initSSO
}

func initSSO(key string) (afero.Fs, storage.Release, error) {
	logger := zap.L()
	settings := &OssSettings{}
	err := viper.UnmarshalKey(key, settings)
	if err != nil {
		logger.Error("Failed to load OSS settings", zap.Error(err))
		return nil, nil, err
	}
	if settings.Endpoint == "" {
		logger.Info("try to load from ENV")
		settings.Bucket = os.Getenv("OSS_BUCKET")
		settings.AccessKey = os.Getenv("OSS_ID")
		settings.SecretKey = os.Getenv("OSS_SECRET")
		settings.Endpoint = os.Getenv("OSS_ENDPOINT")
		settings.Region = os.Getenv("OSS_REGION")
	}
	if settings.Bucket == "" || settings.AccessKey == "" || settings.SecretKey == "" || settings.Endpoint == "" || settings.Region == "" {
		logger.Error("OSS config missed, use regular file instead")
		return nil, nil, errors.New("settings missed")
	}
	logger.Info("going to connect to oss", zap.String("endpoint", settings.Endpoint), zap.String("bucket", settings.Bucket), zap.String("path", settings.Path))

	ossfs, err := fsoss.NewOssFs(settings.Endpoint, settings.AccessKey, settings.SecretKey, settings.Bucket)
	if err != nil {
		logger.Error("Failed to create OSS filesystem", zap.Error(err))
		return nil, nil, err
	}

	fs := afero.NewBasePathFs(ossfs, settings.Path)
	logger.Info("ossfs created", zap.String("bucket", settings.Bucket), zap.String("prefix", settings.Path))

	return fs, func() {}, nil
}
