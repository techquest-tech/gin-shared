package ginshared

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/unrolled/secure"
	"go.uber.org/zap"
)

type Tlssettings struct {
	Enabled bool
	Pem     string
	Key     string
}

func init() {
	Provide(CheckAndSetupTLS)
}

func CheckAndSetupTLS(logger *zap.Logger) (tls *Tlssettings) {
	tls = &Tlssettings{}
	settings := viper.Sub("tls")
	if settings == nil {
		logger.Info("TLS is not enabled")
		return
	}
	settings.Unmarshal(tls)
	if tls.Key != "" && tls.Pem != "" {
		tls.Enabled = true
		logger.Info("TLS is enabled.")
	}
	return
}

func (s *Tlssettings) Middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		mw := secure.New(secure.Options{
			SSLRedirect: true,
		})
		err := mw.Process(ctx.Writer, ctx.Request)
		if err != nil {
			zap.L().Warn("tls error", zap.Error(err))
			return
		}
		ctx.Next()
	}
}
