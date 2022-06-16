package cors

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func initCors(logger *zap.Logger, router *gin.Engine) ginshared.DiController {
	logger.Info("CORS module loaded.")
	corsSettings := viper.Sub("CORS")
	if corsSettings != nil {
		enabled := corsSettings.GetBool("enabled")
		if enabled {
			router.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"*"},
				AllowMethods:     []string{"*"},
				AllowHeaders:     []string{"*"},
				ExposeHeaders:    []string{"*"},
				AllowCredentials: true,
				MaxAge:           12 * time.Hour,
			}))
			logger.Info("CORS enabled, defaults allow all")
		}
	}

	return true
}

func init() {
	ginshared.ProvideController(initCors)
}
