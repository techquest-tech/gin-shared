package storage

import (
	"errors"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	localcache "github.com/patrickmn/go-cache"
	"github.com/rs/xid"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

const (
	localResourceTTL       = 24 * time.Hour
	localResourceRouteName = "resources"
)

var (
	localResourcePathByXID = localcache.New(localResourceTTL, 2*localResourceTTL)
	localResourceXIDByPath = localcache.New(localResourceTTL, 2*localResourceTTL)
)

// PublicResourceEntry 定义公开资源缓存条目。
// Key: 存储配置键。
// FullFileName: 调用方传入的完整文件名。
type PublicResourceEntry struct {
	Key          string
	FullFileName string
}

// PublicResourceComponent 注册文件资源公开访问路由。
// 负责按 xid 查找缓存中的资源描述，并通过 afero.Fs 返回文件内容。
type PublicResourceComponent struct {
	ginshared.DefaultComponent
}

func init() {
	PublicURLFuncFactories[""] = createFSPublicURL
	PublicURLFuncFactories["local"] = createFSPublicURL
	PublicURLFuncFactories["sftp"] = createFSPublicURL
	ginshared.RegisterComponent(&PublicResourceComponent{})
}

// OnEngineInited 在 Gin 引擎初始化完成后注册资源路由。
// r: Gin 路由引擎。
// 返回值：返回注册过程中的错误信息。
func (c *PublicResourceComponent) OnEngineInited(r *gin.Engine) error {
	uri := path.Join(ginshared.GetbaseUrl(), localResourceRouteName, ":xid")
	zap.L().Info("[storage] register public resource route", zap.String("uri", uri))
	r.GET(uri, serveLocalResourceByXID)
	return nil
}

// createFSPublicURL 为基于 FS 的文件返回公共 URL 生成函数。
// key: 存储配置键。
// 返回值：返回基于当前 key 的公共 URL 生成函数。
func createFSPublicURL(key string) PublicURLFunc {
	return func(fullFileName string) (string, error) {
		logger := zap.L()
		fullFileName = normalizeStoredFullFileName(fullFileName)
		if fullFileName == "" {
			err := errors.New("full file name is empty")
			logger.Error("[storage] create fs public url failed",
				zap.String("key", key),
				zap.String("fullFileName", fullFileName),
				zap.Error(err),
			)
			return "", err
		}
		resourcePath := resolveFSResourcePath(key, fullFileName)
		if resourcePath == "" {
			err := errors.New("resource path is empty")
			logger.Error("[storage] create fs public url failed",
				zap.String("key", key),
				zap.String("fullFileName", fullFileName),
				zap.Error(err),
			)
			return "", err
		}

		fs, release, err := CreateFs(key)
		if err != nil {
			logger.Error("[storage] create fs public url failed",
				zap.String("key", key),
				zap.String("fullFileName", fullFileName),
				zap.String("resourcePath", resourcePath),
				zap.Error(err),
			)
			return "", err
		}
		defer release()

		info, err := fs.Stat(resourcePath)
		if err != nil || info.IsDir() {
			logger.Error("[storage] create fs public url failed",
				zap.String("key", key),
				zap.String("fullFileName", fullFileName),
				zap.String("resourcePath", resourcePath),
				zap.Error(err),
			)
			if err != nil {
				return "", err
			}
			return "", errors.New("resource is directory")
		}

		cacheKey := buildResourceCacheKey(key, fullFileName)
		entry := PublicResourceEntry{
			Key:          key,
			FullFileName: fullFileName,
		}

		if rawXID, found := localResourceXIDByPath.Get(cacheKey); found {
			resourceXID, _ := rawXID.(string)
			localResourcePathByXID.Set(resourceXID, entry, localResourceTTL)
			localResourceXIDByPath.Set(cacheKey, resourceXID, localResourceTTL)
			publicURL := buildLocalResourceURL(resourceXID)
			logger.Info("[storage] reuse fs public url",
				zap.String("key", key),
				zap.String("fullFileName", fullFileName),
				zap.String("resourcePath", resourcePath),
				zap.String("xid", resourceXID),
				zap.String("publicURL", publicURL),
			)
			return publicURL, nil
		}

		resourceXID := xid.New().String()
		localResourcePathByXID.Set(resourceXID, entry, localResourceTTL)
		localResourceXIDByPath.Set(cacheKey, resourceXID, localResourceTTL)
		publicURL := buildLocalResourceURL(resourceXID)
		logger.Info("[storage] create fs public url done",
			zap.String("key", key),
			zap.String("fullFileName", fullFileName),
			zap.String("resourcePath", resourcePath),
			zap.String("xid", resourceXID),
			zap.Duration("ttl", localResourceTTL),
			zap.String("publicURL", publicURL),
		)
		return publicURL, nil
	}
}

// buildLocalResourceURL 构建本地资源公开访问 URL。
// resourceXID: 资源访问 xid。
// 返回值：返回绝对 URL 或相对 URI。
func buildLocalResourceURL(resourceXID string) string {
	uri := path.Join(ginshared.GetbaseUrl(), localResourceRouteName, resourceXID)
	domain := strings.TrimSpace(viper.GetString("domain"))
	if domain == "" {
		return uri
	}
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return strings.TrimRight(domain, "/") + uri
	}
	if strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "localhost") {
		return "http://" + strings.TrimRight(domain, "/") + uri
	}
	return "https://" + strings.TrimRight(domain, "/") + uri
}

// serveLocalResourceByXID 根据 xid 返回本地文件。
// c: Gin 请求上下文。
func serveLocalResourceByXID(c *gin.Context) {
	logger := zap.L()
	resourceXID := strings.TrimSpace(c.Param("xid"))
	start := time.Now()
	logger.Info("[storage] serve local resource start", zap.String("xid", resourceXID))

	rawPath, found := localResourcePathByXID.Get(resourceXID)
	if !found {
		logger.Warn("[storage] local resource xid expired or not found",
			zap.String("xid", resourceXID),
			zap.Duration("cost", time.Since(start)),
		)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	entry, _ := rawPath.(PublicResourceEntry)
	fullFileName := normalizeStoredFullFileName(entry.FullFileName)
	resourcePath := resolveFSResourcePath(entry.Key, fullFileName)
	if fullFileName == "" || resourcePath == "" {
		logger.Warn("[storage] local resource path invalid",
			zap.String("xid", resourceXID),
			zap.String("key", entry.Key),
			zap.String("fullFileName", fullFileName),
			zap.Duration("cost", time.Since(start)),
		)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	fs, release, err := CreateFs(entry.Key)
	if err != nil {
		logger.Error("[storage] open fs for resource failed",
			zap.String("xid", resourceXID),
			zap.String("key", entry.Key),
			zap.String("fullFileName", fullFileName),
			zap.String("resourcePath", resourcePath),
			zap.Duration("cost", time.Since(start)),
			zap.Error(err),
		)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer release()

	info, err := fs.Stat(resourcePath)
	if err != nil || info.IsDir() {
		logger.Warn("[storage] fs resource file missing",
			zap.String("xid", resourceXID),
			zap.String("key", entry.Key),
			zap.String("fullFileName", fullFileName),
			zap.String("resourcePath", resourcePath),
			zap.Duration("cost", time.Since(start)),
			zap.Error(err),
		)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	file, err := fs.Open(resourcePath)
	if err != nil {
		logger.Error("[storage] open resource file failed",
			zap.String("xid", resourceXID),
			zap.String("key", entry.Key),
			zap.String("fullFileName", fullFileName),
			zap.String("resourcePath", resourcePath),
			zap.Duration("cost", time.Since(start)),
			zap.Error(err),
		)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer file.Close()

	contentType := mime.TypeByExtension(path.Ext(fullFileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	logger.Info("[storage] serve fs resource done",
		zap.String("xid", resourceXID),
		zap.String("key", entry.Key),
		zap.String("fullFileName", fullFileName),
		zap.String("resourcePath", resourcePath),
		zap.Int64("size", info.Size()),
		zap.String("contentType", contentType),
		zap.Duration("cost", time.Since(start)),
	)
	http.ServeContent(c.Writer, c.Request, path.Base(fullFileName), info.ModTime(), file)
}

// normalizeStoredFullFileName 统一清洗缓存中的完整文件名。
// fullFileName: 原始完整文件名。
// 返回值：返回标准化后的文件名。
func normalizeStoredFullFileName(fullFileName string) string {
	fullFileName = strings.TrimSpace(fullFileName)
	if fullFileName == "" {
		return ""
	}
	cleaned := filepath.Clean(fullFileName)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

// buildResourceCacheKey 生成资源缓存键。
// key: 存储配置键。
// fullFileName: 完整文件名。
// 返回值：返回缓存键。
func buildResourceCacheKey(key, fullFileName string) string {
	return strings.TrimSpace(key) + "::" + normalizeStoredFullFileName(fullFileName)
}

// resolveFSResourcePath 将完整文件名转换为 FS 可访问的相对路径。
// key: 存储配置键。
// fullFileName: 完整文件名。
// 返回值：返回相对 FS 根目录的路径。
func resolveFSResourcePath(key, fullFileName string) string {
	fullFileName = normalizeStoredFullFileName(fullFileName)
	if fullFileName == "" {
		return ""
	}

	rootPath := normalizeStoredFullFileName(viper.GetString(key + ".path"))
	if rootPath != "" {
		rel, err := filepath.Rel(rootPath, fullFileName)
		if err == nil {
			rel = filepath.ToSlash(rel)
			if rel != "." && !strings.HasPrefix(rel, "../") && rel != ".." {
				return strings.TrimPrefix(rel, "/")
			}
		}
	}

	return strings.TrimPrefix(filepath.ToSlash(fullFileName), "/")
}
