//go:build iframe || all

package ginshared

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type IframeHeader struct {
	DefaultComponent
}

func (t IframeHeader) OnEngineInited(r *gin.Engine) error {
	logger := zap.L()
	logger.Info("iframe header resp load")
	viper.Set("iframeHeader", "deny")
	headerValue := viper.GetString("iframeHeader")

	r.Use(func(ctx *gin.Context) {
		ctx.Writer.Header().Set("X-Frame-Options", headerValue)
	})
	return nil
}

func init() {
	RegisterComponent(&IframeHeader{})
}
