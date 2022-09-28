//go:build all || cors

package ginshared

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type CorsComponent struct {
	core.DefaultComponent
	Enabled bool
}

// func init() {
// 	core.RegisterComponent(&CorsComponent{})
// }

func (c CorsComponent) OnEngineInited(r *gin.Engine) error {
	log := zap.L()
	corsSettings := viper.Sub("CORS")
	if corsSettings != nil {
		corsSettings.Unmarshal(c)
		if c.Enabled {
			r.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"*"},
				AllowMethods:     []string{"*"},
				AllowHeaders:     []string{"*"},
				ExposeHeaders:    []string{"*"},
				AllowCredentials: true,
				MaxAge:           12 * time.Hour,
			}))
			log.Info("CORS enabled, defaults allow all")
		}
	} else {
		log.Info("CORS is disabled.")
	}
	return nil
}

func init() {
	core.RegisterComponent(&CorsComponent{})
}
