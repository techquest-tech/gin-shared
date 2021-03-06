package ginshared

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

const (
	KeyAddress = "address"
	KeyInitDB  = "database.initDB"
)

// var PreStarterOptions = dig.Group("PreStarter")

func initEngine(logger *zap.Logger) *gin.Engine {

	router := gin.New()
	router.Use(ginzap.Ginzap(logger, time.RFC3339, false))
	router.Use(ginzap.RecoveryWithZap(logger, true))
	logger.Info("router engine inited.")

	// if viper.GetBool("prometheus.enabled") {
	// 	p := ginprom.New(
	// 		ginprom.Engine(router),
	// 		ginprom.Subsystem("gin"),
	// 		ginprom.Path("/metrics"),
	// 	)
	// 	router.Use(p.Instrument())
	// 	logger.Info("prometheus module enabled.")
	// }

	//check CORS settings
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
	return router
}

func initBasedRouterGroup(logger *zap.Logger, router *gin.Engine) *gin.RouterGroup {
	base := viper.GetString("baseUri")
	return router.Group(base)
}

func init() {

	core.Container.Provide(core.InitLogger)

	core.Container.Provide(initEngine)

	core.Container.Provide(initBasedRouterGroup)
}

type Params struct {
	dig.In
	Logger      *zap.Logger
	Router      *gin.Engine
	Controllers []DiController `group:"controllers"`
}

func Start() error {
	// core.Container.Provide(NewService)
	err := core.Container.Invoke(func(p Params) error {

		viper.SetDefault(KeyAddress, ":5000")

		address := viper.GetString(KeyAddress)

		if len(p.Controllers) == 0 {
			return fmt.Errorf("no controller available")
		}

		err := p.Router.Run(address)
		if err != nil {
			log.Fatalln("run app failed. ", err)
			return err
		}

		p.Logger.Info("app is stopping")
		p.Logger.Info("stopped.")
		return nil
	})
	if err != nil {
		panic(err)
	}
	return err
}
