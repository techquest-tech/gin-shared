package storage

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/techquest-tech/fsoss"
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
	FSFactories["oss"] = initSSO
	PublicURLFuncFactories["oss"] = createOSSPublicURL
}

func initSSO(key string) (afero.Fs, Release, error) {
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

	release := func() {}

	logger.Info("ossfs created", zap.String("bucket", settings.Bucket), zap.String("prefix", settings.Path))
	if settings.Path == "" {
		return ossfs, release, nil
	}
	fs := afero.NewBasePathFs(ossfs, settings.Path)

	return fs, release, nil
}

// createOSSPublicURL 为 OSS 文件返回公开访问 URL 生成函数。
// key: 存储配置键。
// 返回值：返回基于当前 key 的公开访问 URL 生成函数。
func createOSSPublicURL(key string) PublicURLFunc {
	return func(fullFileName string) (string, error) {
		logger := zap.L()
		settings := &OssSettings{}
		if err := viper.UnmarshalKey(key, settings); err != nil {
			logger.Error("[storage] load oss settings for public url failed", zap.String("key", key), zap.Error(err))
			return "", err
		}
		if settings.Endpoint == "" {
			settings.Bucket = os.Getenv("OSS_BUCKET")
			settings.AccessKey = os.Getenv("OSS_ID")
			settings.SecretKey = os.Getenv("OSS_SECRET")
			settings.Endpoint = os.Getenv("OSS_ENDPOINT")
			settings.Region = os.Getenv("OSS_REGION")
		}

		fullFileName = strings.TrimSpace(fullFileName)
		if fullFileName == "" {
			err := fmt.Errorf("full file name is empty")
			logger.Error("[storage] create oss public url failed", zap.String("key", key), zap.Error(err))
			return "", err
		}
		if settings.Bucket == "" || settings.Endpoint == "" {
			err := fmt.Errorf("oss public url requires bucket and endpoint")
			logger.Error("[storage] create oss public url failed",
				zap.String("key", key),
				zap.String("bucket", settings.Bucket),
				zap.String("endpoint", settings.Endpoint),
				zap.Error(err),
			)
			return "", err
		}

		endpoint := strings.TrimSpace(settings.Endpoint)
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
		parsed, err := url.Parse(endpoint)
		if err != nil {
			logger.Error("[storage] parse oss endpoint failed",
				zap.String("key", key),
				zap.String("endpoint", endpoint),
				zap.Error(err),
			)
			return "", err
		}

		host := parsed.Host
		if !strings.HasPrefix(host, settings.Bucket+".") {
			host = settings.Bucket + "." + host
		}
		parsed.Host = host
		parsed.Path = path.Join("/", strings.TrimPrefix(fullFileName, "/"))
		publicURL := parsed.String()
		logger.Info("[storage] create oss public url done",
			zap.String("key", key),
			zap.String("bucket", settings.Bucket),
			zap.String("fullFileName", fullFileName),
			zap.String("publicURL", publicURL),
		)
		return publicURL, nil
	}
}
