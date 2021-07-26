package gin

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Depado/ginprom"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

const (
	KeyAddress = "address"
)

var ControllerOptions = dig.Group("controllers")

func initLogger() *zap.Logger {

	logger := zap.NewExample()

	viper.SetDefault(KeyAddress, ":5000")

	viper.SetConfigName("app")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("/etc/gin")
	viper.AddConfigPath("$HOME/.gin")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	envfile := os.Getenv("ENV")
	if envfile != "" {
		profileConfig := viper.New()
		profileConfig.SetConfigName(envfile)
		profileConfig.SetConfigType("yaml")
		profileConfig.AddConfigPath("config")
		profileConfig.AddConfigPath("../config")
		err := profileConfig.ReadInConfig()
		if err != nil {
			logger.Error("error while load env profile",
				zap.String("env", envfile),
				zap.Any("error", err),
			)
			panic(err)
		}
		result := profileConfig.AllSettings()
		viper.MergeConfigMap(result)
		logger.Debug("env profile loaded.", zap.Any("result", result), zap.String("env", envfile))
	} else {
		logger.Info("no env profiled found.")
	}

	logger.Info("Config loaded.")
	return logger
}

func initEngine(logger *zap.Logger) *gin.Engine {
	router := gin.New()
	router.Use(ginzap.Ginzap(logger, time.RFC3339, false))
	router.Use(ginzap.RecoveryWithZap(logger, true))
	logger.Info("router engine inited.")

	if viper.GetBool("prometheus.enabled") {
		p := ginprom.New(
			ginprom.Engine(router),
			ginprom.Subsystem("gin"),
			ginprom.Path("/metrics"),
		)
		router.Use(p.Instrument())
		logger.Info("prometheus module enabled.")
	}
	return router
}

func initBasedRouterGroup(logger *zap.Logger, router *gin.Engine) *gin.RouterGroup {
	base := viper.GetString("baseUri")
	return router.Group(base)
}

func init() {

	container.Provide(initLogger)

	container.Provide(initEngine)

	container.Provide(initBasedRouterGroup)
}

type Params struct {
	dig.In
	Logger      *zap.Logger
	Router      *gin.Engine
	Controllers []DiController `group:"controllers"`
}

func Start() error {
	// container.Provide(NewService)
	err := container.Invoke(func(p Params) error {

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
