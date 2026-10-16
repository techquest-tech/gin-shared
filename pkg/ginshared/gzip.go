package ginshared

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type GzipEnabled struct {
	DefaultComponent
}

// defaultGzipMinLength 小于该长度的响应不压缩：实测 <~100B 时 gzip 反而更大（71B -> 96B），256 留足余量。
const defaultGzipMinLength = 256

// defaultGzipExcludedExtensions 列出「再压一遍也基本不省字节、只花 CPU」的类型。
// 注意 WithExcludedExtensions 是整体替换库默认值（库默认含 .jpeg/.gif/.png/.jpg），不要漏项。
var defaultGzipExcludedExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico",
	".mp4", ".mp3", ".zip", ".gz", ".br", ".7z", ".rar", ".xz",
	".pdf", ".woff", ".woff2", ".ttf", ".eot",
}

// OnEngineInited 注册 gzip 压缩中间件。
//
// 可配置项（viper，均可用环境变量覆盖，如 GZIP_MINLENGTH=512）：
//
//	gzip.enabled              bool     是否启用，默认 true
//	gzip.level                int      默认 -1 = DefaultCompression；1..9 为具体级别（越大越费 CPU，9 收益很小）
//	gzip.minLength            int      默认 256，小于该长度的响应不压
//	gzip.excludedExtensions   []string 默认 defaultGzipExcludedExtensions（整体替换，不要漏项）
//	gzip.excludedPaths        []string 默认空，按请求路径前缀排除
//	gzip.excludedPathRegexps  []string 默认空，按请求路径正则排除（适合无后缀的导出/下载接口）
func (*GzipEnabled) OnEngineInited(r *gin.Engine) error {
	if viper.IsSet("gzip.enabled") && !viper.GetBool("gzip.enabled") {
		zap.L().Info("gzip middleware disabled by config.", zap.String("key", "gzip.enabled"))
		return nil
	}

	// DefaultCompression(6)：实测 600KB 列表 JSON 上 level 9 只多省 4.3% 字节、CPU 却是 8 倍。
	level := gzip.DefaultCompression
	if viper.IsSet("gzip.level") {
		if lv := viper.GetInt("gzip.level"); isGzipLevelValid(lv) {
			level = lv
		} else {
			zap.L().Warn("invalid gzip.level, fallback to DefaultCompression.",
				zap.Int("level", lv), zap.Int("fallback", gzip.DefaultCompression))
		}
	}

	minLength := defaultGzipMinLength
	if viper.IsSet("gzip.minLength") {
		if ml := viper.GetInt("gzip.minLength"); ml >= 0 {
			minLength = ml
		} else {
			zap.L().Warn("invalid gzip.minLength, fallback to default.",
				zap.Int("minLength", ml), zap.Int("fallback", defaultGzipMinLength))
		}
	}

	excludedExtensions := defaultGzipExcludedExtensions
	if viper.IsSet("gzip.excludedExtensions") {
		excludedExtensions = viper.GetStringSlice("gzip.excludedExtensions")
	}

	excludedPaths := viper.GetStringSlice("gzip.excludedPaths")
	excludedPathRegexps := viper.GetStringSlice("gzip.excludedPathRegexps")

	options := []gzip.Option{
		gzip.WithMinLength(minLength),
		gzip.WithExcludedExtensions(excludedExtensions),
	}
	if len(excludedPaths) > 0 {
		options = append(options, gzip.WithExcludedPaths(excludedPaths))
	}
	if len(excludedPathRegexps) > 0 {
		options = append(options, gzip.WithExcludedPathsRegexs(excludedPathRegexps))
	}

	r.Use(gzip.Gzip(level, options...))

	// 打出来便于从 pod 日志回读生效值：kubectl logs <pod> | grep "gzip middleware enabled"
	zap.L().Info("gzip middleware enabled.",
		zap.Int("level", level),
		zap.Int("minLength", minLength),
		zap.Strings("excludedExtensions", excludedExtensions),
		zap.Strings("excludedPaths", excludedPaths),
		zap.Strings("excludedPathRegexps", excludedPathRegexps),
	)
	return nil
}

// isGzipLevelValid 与 gin-contrib/gzip 内部的校验保持一致，避免非法级别被静默改成 4。
func isGzipLevelValid(level int) bool {
	return level == gzip.DefaultCompression ||
		level == gzip.NoCompression ||
		(level >= gzip.BestSpeed && level <= gzip.BestCompression)
}

func init() {
	RegisterComponent(&GzipEnabled{})
}
