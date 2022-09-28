//go:build iframe
// +build iframe

package ginshared

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type IframeHeader struct {
	core.DefaultComponent
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
	core.RegisterComponent(&IframeHeader{})
}
