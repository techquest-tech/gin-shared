//go:build !disableCORS || all

package ginshared

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CorsComponent struct {
	DefaultComponent
	// Enabled bool
}

// func init() {
// 	core.RegisterComponent(&CorsComponent{})
// }

func (c CorsComponent) OnEngineInited(r *gin.Engine) error {
	log := zap.L()
	// corsSettings := viper.Sub("CORS")
	// if corsSettings != nil {
	// 	corsSettings.Unmarshal(c)
	// 	if c.Enabled {
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"*"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"*"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	log.Info("CORS enabled, defaults allow all")
	// 	}
	// } else {
	// 	log.Info("CORS is disabled.")
	// }
	return nil
}

func init() {
	RegisterComponent(&CorsComponent{})
}
