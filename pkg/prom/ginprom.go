package prom

import (
	"github.com/Depado/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func enabledGinprom(logger *zap.Logger, router *gin.Engine) ginshared.DiController {
	logger.Info("Gin prometheus module loaded.")
	if viper.GetBool("prometheus.enabled") {
		p := ginprom.New(
			ginprom.Engine(router),
			ginprom.Subsystem("gin"),
			ginprom.Path("/metrics"),
		)
		router.Use(p.Instrument())
		logger.Info("prometheus module enabled.")
	}
	return true
}

func init() {
	ginshared.ProvideController(enabledGinprom)
}
