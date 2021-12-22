package ginshared

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Depado/ginprom"
	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

const (
	KeyAddress = "address"
	KeyInitDB  = "database.initDB"
)

var AppName = "app"

var ControllerOptions = dig.Group("controllers")

// var PreStarterOptions = dig.Group("PreStarter")

func InitConfig() {

	appname := os.Getenv("APP_NAME")
	if appname != "" {
		AppName = appname
		fmt.Printf("user AppName = %s", appname)
	}

	viper.SetConfigName(AppName)
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
			// logger.Error("error while load env profile",
			// 	zap.String("env", envfile),
			// 	zap.Any("error", err),
			// )
			log.Fatalf("error while load env profile %s. %v", envfile, err)
			panic(err)
		}
		result := profileConfig.AllSettings()
		viper.MergeConfigMap(result)
		// logger.Debug("env profile loaded.", zap.Any("result", result), zap.String("env", envfile))
		log.Printf("env profile %s loaded", envfile)
	}

	log.Print("load config done.")
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
