package ginshared

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	KeyAddress  = "address"
	KeyShutdown = "shutdown"
	KeyInitDB   = "database.initDB"
)

func GetbaseUrl() string {
	viper.SetDefault("baseUri", "/v1")
	return viper.GetString("baseUri")
}

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
	core.Container.Provide(initEngine)
	core.Container.Provide(initBasedRouterGroup)
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

func GetFullUrl(c *gin.Context) string {
	domain := viper.GetString("domain")
	if domain == "" {
		domain = c.Request.Host
	}
	s := "http"
	if c.Request.TLS != nil || (strings.Contains(domain, ":443") || !strings.HasPrefix(domain, "127.0.0.1")) {
		s = "https"
	}
	return fmt.Sprintf("%s://%s", s, domain)
}

func Start() error {
	// core.Container.Provide(NewService)
	return core.Container.Invoke(func(p Params) (err error) {
		viper.SetDefault(KeyAddress, ":5001")
		viper.SetDefault(KeyShutdown, 3*time.Second)

		// check if have static folder, if yes, enabled the static router
		info, err := os.Stat("static")
		if err == nil && info.IsDir() {
			p.Router.Static("/", "static")
			p.Logger.Info("static router enabled.")
		}

		address := viper.GetString(KeyAddress)
		shutdownDur := viper.GetDuration(KeyShutdown)

		logger := p.Logger.With(zap.String("address", address))

		if len(p.Controllers) == 0 {
			logger.Error("no controllers defined.")

			return fmt.Errorf("no controller available")
		}

		core.NotifyStarted()

		svc := &http.Server{
			Addr:    address,
			Handler: p.Router,
		}

		core.OnServiceStopping(func() {
			ctx, cancel := context.WithTimeout(context.TODO(), shutdownDur)

			defer cancel()
			if err := svc.Shutdown(ctx); err != nil {
				logger.Fatal("Server Shutdown failed", zap.Error(err))
			}
			// catching ctx.Done(). timeout of 5 seconds.
			// select {
			<-ctx.Done()
			logger.Info("shutdown timeout", zap.Duration("dur", shutdownDur))
			// }
			logger.Info("stopped.")
		})

		go func() {
			logger.Info("gin service starting ", zap.String("addr", address))
			if p.Tls.Enabled {
				err = svc.ListenAndServeTLS(p.Tls.Pem, p.Tls.Key)
			} else {
				err = svc.ListenAndServe()
			}
			if err != nil && err != http.ErrServerClosed {
				logger.Fatal("start gin service failed.", zap.Error(err))
			}

			logger.Info("app is stopping")
		}()

		core.CloseOnlyNotified()

		return nil
	})
}
