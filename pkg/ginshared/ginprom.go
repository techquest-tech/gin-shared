// go:build prom || all

package ginshared

import (
	"github.com/Depado/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Prom struct {
	DefaultComponent
}

func (p Prom) OnEngineInited(r *gin.Engine) error {
	logger := zap.L()
	logger.Info("Gin prometheus module loaded.")
	if viper.GetBool("prometheus.enabled") {
		p := ginprom.New(
			ginprom.Engine(r),
			ginprom.Subsystem("gin"),
			ginprom.Path("/metrics"),
		)
		r.Use(p.Instrument())
		logger.Info("prometheus module enabled.")
	}
	return nil
}

func init() {
	RegisterComponent(&Prom{})
}
