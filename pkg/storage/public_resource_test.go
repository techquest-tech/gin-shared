package storage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// TestCreateOSSPublicURL 验证 OSS 公共 URL 会基于 bucket、endpoint 和完整文件名正确拼接。
func TestCreateOSSPublicURL(t *testing.T) {
	const rootKey = "storage.test.oss"
	t.Cleanup(func() {
		viper.Set(rootKey, nil)
	})

	viper.Set(rootKey+".type", "oss")
	viper.Set(rootKey+".bucket", "demo-bucket")
	viper.Set(rootKey+".endpoint", "https://oss-cn-hangzhou.aliyuncs.com")

	publicURLFunc, err := GetPublicURLFunc(rootKey)
	require.NoError(t, err)

	publicURL, err := CreatePublicURLWithFunc(publicURLFunc, "upload/owner-a/blockedEpc/202607/epc.png")
	require.NoError(t, err)
	require.Equal(t, "https://demo-bucket.oss-cn-hangzhou.aliyuncs.com/upload/owner-a/blockedEpc/202607/epc.png", publicURL)
}

// TestCreateOSSPublicURLWithInternalEndpoint 验证 OSS 内网 Endpoint 会自动切换为外网地址。
func TestCreateOSSPublicURLWithInternalEndpoint(t *testing.T) {
	const rootKey = "storage.test.oss.internal"
	t.Cleanup(func() {
		viper.Set(rootKey, nil)
	})

	viper.Set(rootKey+".type", "oss")
	viper.Set(rootKey+".bucket", "demo-bucket")
	viper.Set(rootKey+".endpoint", "https://oss-cn-hangzhou-internal.aliyuncs.com")

	publicURLFunc, err := GetPublicURLFunc(rootKey)
	require.NoError(t, err)

	publicURL, err := CreatePublicURLWithFunc(publicURLFunc, "upload/owner-a/blockedEpc/202607/epc.png")
	require.NoError(t, err)
	require.Equal(t, "https://demo-bucket.oss-cn-hangzhou.aliyuncs.com/upload/owner-a/blockedEpc/202607/epc.png", publicURL)
}

// TestNormalizeOSSEndpoint 验证阿里云 OSS 内网域名会被规范化为外网域名。
func TestNormalizeOSSEndpoint(t *testing.T) {
	normalizedEndpoint, converted, err := normalizeOSSEndpoint("oss-cn-hangzhou-internal.aliyuncs.com")
	require.NoError(t, err)
	require.True(t, converted)
	require.Equal(t, "oss-cn-hangzhou.aliyuncs.com", normalizedEndpoint)

	normalizedEndpoint, converted, err = normalizeOSSEndpoint("https://oss-cn-hangzhou.aliyuncs.com")
	require.NoError(t, err)
	require.False(t, converted)
	require.Equal(t, "https://oss-cn-hangzhou.aliyuncs.com", normalizedEndpoint)
}

// TestLoadOSSPublicEndpointFromEnv 验证公共 URL 会优先读取公网 Endpoint。
func TestLoadOSSPublicEndpointFromEnv(t *testing.T) {
	t.Setenv("OSS_ENDPOINT_PUB", "https://oss-cn-hangzhou.aliyuncs.com")
	t.Setenv("OSS_ENDPOINT", "https://oss-cn-hangzhou-internal.aliyuncs.com")
	require.Equal(t, "https://oss-cn-hangzhou.aliyuncs.com", loadOSSPublicEndpointFromEnv())

	t.Setenv("OSS_ENDPOINT_PUB", "")
	require.Equal(t, "https://oss-cn-hangzhou-internal.aliyuncs.com", loadOSSPublicEndpointFromEnv())
}

// TestCreatePublicURLDefault 验证默认键会使用 fileroot 配置生成公共 URL。
func TestCreatePublicURLDefault(t *testing.T) {
	t.Cleanup(func() {
		viper.Set("fileroot", nil)
	})

	viper.Set("fileroot.type", "oss")
	viper.Set("fileroot.bucket", "default-bucket")
	viper.Set("fileroot.endpoint", "https://oss-cn-shanghai.aliyuncs.com")

	publicURL, err := CreatePublicURLDefault("upload/default/epc.png")
	require.NoError(t, err)
	require.Equal(t, "https://default-bucket.oss-cn-shanghai.aliyuncs.com/upload/default/epc.png", publicURL)
}

// TestCreateFSPublicURLAndServe 验证默认 FS 公共 URL 会生成可重复使用的 xid，并可通过公开路由访问。
func TestCreateFSPublicURLAndServe(t *testing.T) {
	localResourcePathByXID.Flush()
	localResourceXIDByPath.Flush()
	gin.SetMode(gin.TestMode)
	const rootKey = "storage.test.local"
	t.Cleanup(func() {
		viper.Set("domain", nil)
		viper.Set("baseUri", nil)
		viper.Set(rootKey, nil)
	})
	viper.Set("baseUri", "/v1")

	rootDir := t.TempDir()
	viper.Set(rootKey+".path", rootDir)
	tmpFile, err := os.CreateTemp(rootDir, "resource-*.txt")
	require.NoError(t, err)
	_, err = tmpFile.WriteString("hello-resource")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	publicURLFunc, err := GetPublicURLFunc(rootKey)
	require.NoError(t, err)

	publicURL1, err := CreatePublicURLWithFunc(publicURLFunc, tmpFile.Name())
	require.NoError(t, err)
	publicURL2, err := CreatePublicURLWithFunc(publicURLFunc, tmpFile.Name())
	require.NoError(t, err)
	require.Equal(t, publicURL1, publicURL2)

	router := gin.New()
	component := &PublicResourceComponent{}
	require.NoError(t, component.OnEngineInited(router))

	req := httptest.NewRequest(http.MethodGet, publicURL1, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "hello-resource", string(body))
}

// TestServeLocalResourceByXID_NotFound 验证 xid 失效或不存在时返回 404。
func TestServeLocalResourceByXID_NotFound(t *testing.T) {
	localResourcePathByXID.Flush()
	localResourceXIDByPath.Flush()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		viper.Set("baseUri", nil)
	})
	viper.Set("baseUri", "/v1")

	router := gin.New()
	component := &PublicResourceComponent{}
	require.NoError(t, component.OnEngineInited(router))

	req := httptest.NewRequest(http.MethodGet, path.Join("/v1/resources", "missing-xid"), nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusNotFound, resp.Code)
}
