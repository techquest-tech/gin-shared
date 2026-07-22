package storage

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
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

const ossPublicURLExpireSeconds int64 = 24 * 60 * 60

func init() {
	FSFactories["oss"] = initSSO
	PublicURLFuncFactories["oss"] = createOSSPublicURL
}

// initSSO 根据配置键初始化 OSS 文件系统。
// key: 存储配置键。
// 返回值：返回 afero.Fs、释放函数和错误信息。
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
		settings.Endpoint = strings.TrimSpace(os.Getenv("OSS_ENDPOINT"))
		settings.Region = os.Getenv("OSS_REGION")
	}
	if settings.Bucket == "" || settings.AccessKey == "" || settings.SecretKey == "" || settings.Endpoint == "" || settings.Region == "" {
		logger.Error("OSS config missed, use regular file instead")
		return nil, nil, errors.New("settings missed")
	}
	normalizedEndpoint, converted, err := normalizeOSSEndpoint(settings.Endpoint)
	if err != nil {
		logger.Error("Failed to normalize OSS endpoint", zap.String("endpoint", settings.Endpoint), zap.Error(err))
		return nil, nil, err
	}
	if converted {
		logger.Info("convert OSS endpoint to public endpoint",
			zap.String("originalEndpoint", settings.Endpoint),
			zap.String("publicEndpoint", normalizedEndpoint),
		)
	}
	settings.Endpoint = normalizedEndpoint
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

// loadOSSPublicEndpointFromEnv 按公共访问场景的优先级读取 OSS Endpoint。
// 返回值：优先返回 `OSS_ENDPOINT_PUB`，未配置时回退到 `OSS_ENDPOINT`。
func loadOSSPublicEndpointFromEnv() string {
	publicEndpoint := strings.TrimSpace(os.Getenv("OSS_ENDPOINT_PUB"))
	if publicEndpoint != "" {
		return publicEndpoint
	}
	return strings.TrimSpace(os.Getenv("OSS_ENDPOINT"))
}

// normalizeOSSEndpoint 将 OSS Endpoint 统一转换为可公网访问的地址。
// endpoint: 原始 OSS Endpoint，允许包含或不包含协议头。
// 返回值：返回归一化后的 Endpoint、是否发生转换以及错误信息。
func normalizeOSSEndpoint(endpoint string) (string, bool, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", false, nil
	}

	hasScheme := strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")
	parseTarget := endpoint
	if !hasScheme {
		parseTarget = "https://" + endpoint
	}

	parsed, err := url.Parse(parseTarget)
	if err != nil {
		return "", false, err
	}
	if parsed.Hostname() == "" {
		return "", false, fmt.Errorf("oss endpoint host is empty")
	}

	normalizedHost, converted := normalizeOSSHost(parsed.Hostname())
	if converted {
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort(normalizedHost, port)
		} else {
			parsed.Host = normalizedHost
		}
	}

	normalizedEndpoint := parsed.String()
	if !hasScheme {
		normalizedEndpoint = strings.TrimPrefix(normalizedEndpoint, parsed.Scheme+"://")
	}
	return normalizedEndpoint, converted, nil
}

// normalizeOSSHost 将阿里云 OSS 内网域名转换为外网域名。
// host: OSS Endpoint 中的主机名部分，不包含协议头。
// 返回值：返回转换后的主机名以及是否发生转换。
func normalizeOSSHost(host string) (string, bool) {
	labels := strings.Split(host, ".")
	for index, label := range labels {
		if strings.HasPrefix(label, "oss-") && strings.HasSuffix(label, "-internal") {
			labels[index] = strings.TrimSuffix(label, "-internal")
			return strings.Join(labels, "."), true
		}
	}
	return host, false
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
			settings.Endpoint = loadOSSPublicEndpointFromEnv()
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
		normalizedEndpoint, converted, err := normalizeOSSEndpoint(settings.Endpoint)
		if err != nil {
			logger.Error("[storage] normalize oss endpoint failed",
				zap.String("key", key),
				zap.String("endpoint", settings.Endpoint),
				zap.Error(err),
			)
			return "", err
		}
		if converted {
			logger.Info("[storage] convert oss endpoint to public endpoint",
				zap.String("key", key),
				zap.String("originalEndpoint", settings.Endpoint),
				zap.String("publicEndpoint", normalizedEndpoint),
			)
		}
		settings.Endpoint = normalizedEndpoint

		objectKey := strings.TrimPrefix(fullFileName, "/")
		if settings.AccessKey != "" && settings.SecretKey != "" {
			signEndpoint := strings.TrimSpace(settings.Endpoint)
			signEndpoint = strings.TrimPrefix(signEndpoint, "https://")
			signEndpoint = strings.TrimPrefix(signEndpoint, "http://")

			// OSS 默认是私有读，本地联调与线上展示都优先返回带有效期的签名 GET URL。
			var ossClient *oss.Client
			ossClient, err = oss.New(signEndpoint, settings.AccessKey, settings.SecretKey)
			if err != nil {
				logger.Error("[storage] create oss client for signed url failed",
					zap.String("key", key),
					zap.String("endpoint", signEndpoint),
					zap.String("bucket", settings.Bucket),
					zap.String("fullFileName", fullFileName),
					zap.Error(err),
				)
				return "", err
			}
			bucket, err := ossClient.Bucket(settings.Bucket)
			if err != nil {
				logger.Error("[storage] get oss bucket for signed url failed",
					zap.String("key", key),
					zap.String("bucket", settings.Bucket),
					zap.String("fullFileName", fullFileName),
					zap.Error(err),
				)
				return "", err
			}
			signedURL, err := bucket.SignURL(objectKey, oss.HTTPGet, ossPublicURLExpireSeconds)
			if err != nil {
				logger.Error("[storage] sign oss public url failed",
					zap.String("key", key),
					zap.String("bucket", settings.Bucket),
					zap.String("fullFileName", fullFileName),
					zap.Int64("expireSeconds", ossPublicURLExpireSeconds),
					zap.Error(err),
				)
				return "", err
			}
			logger.Info("[storage] create signed oss public url done",
				zap.String("key", key),
				zap.String("bucket", settings.Bucket),
				zap.String("fullFileName", fullFileName),
				zap.Int64("expireSeconds", ossPublicURLExpireSeconds),
				zap.Time("expireAt", time.Now().Add(time.Duration(ossPublicURLExpireSeconds)*time.Second)),
				zap.String("publicURL", signedURL),
			)
			return signedURL, nil
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
		parsed.Path = path.Join("/", objectKey)
		publicURL := parsed.String()
		logger.Warn("[storage] create unsigned oss public url done",
			zap.String("key", key),
			zap.String("bucket", settings.Bucket),
			zap.String("fullFileName", fullFileName),
			zap.String("publicURL", publicURL),
		)
		return publicURL, nil
	}
}
