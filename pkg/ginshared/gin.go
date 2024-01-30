package ginshared

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/asaskevich/EventBus"
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

func initEngine(logger *zap.Logger, bus EventBus.Bus, p *Components,
	tls *Tlssettings) *gin.Engine {

	router := gin.New()
	router.Use(ginzap.Ginzap(logger, time.RFC3339, false))
	router.Use(ginzap.RecoveryWithZap(logger, true))

	if tls.Enabled {
		router.Use(tls.Middleware())
	}

	// prom.Prom(logger, router)

	p.InitAll(router)

	bus.Publish(core.EventInit, router)

	logger.Info("router engine inited.")

	return router
}

func initBasedRouterGroup(logger *zap.Logger, router *gin.Engine) *gin.RouterGroup {
	base := viper.GetString("baseUri")
	return router.Group(base)
}

func init() {

	// core.Container.Provide(core.InitLogger)

	core.Container.Provide(initEngine)

	core.Container.Provide(initBasedRouterGroup)
	// core.Container.Provide(tracing.NewTracingRequestService)
	// core.RegisterComponent(&cors.CorsComponent{})
	// core.RegisterComponent(&prom.Prom{})
}

type Params struct {
	dig.In
	Logger      *zap.Logger
	Router      *gin.Engine
	Bus         EventBus.Bus
	Tls         *Tlssettings
	Startups    []core.Startup `group:"startups"`
	Controllers []DiController `group:"controllers"`
}

func PrintVersion() {
	zap.L().Info("Application info:", zap.String("appName", core.AppName),
		zap.String("verion", core.Version),
		zap.String("Go version", runtime.Version()),
	)
}
func Start() error {
	// core.Container.Provide(NewService)
	err := core.Container.Invoke(func(p Params) (err error) {
		PrintVersion()
		viper.SetDefault(KeyAddress, ":5001")

		address := viper.GetString(KeyAddress)

		if len(p.Controllers) == 0 {
			return fmt.Errorf("no controller available")
		}

		core.NotifyStarted()
		if p.Tls.Enabled {
			err = p.Router.RunTLS(address, p.Tls.Pem, p.Tls.Key)
		} else {
			err = p.Router.Run(address)
		}

		if err != nil {
			log.Fatalln("run app failed. ", err)
			return err
		}

		p.Logger.Info("app is stopping")
		core.NotifyStopping()
		p.Logger.Info("stopped.")
		return nil
	})
	if err != nil {
		panic(err)
	}
	return err
}
