package prom

import (
	"github.com/Depado/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func Prom(logger *zap.Logger, router *gin.Engine) {
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
}

// func init() {
// 	ginshared.ProvideController(enabledGinprom)
// }
