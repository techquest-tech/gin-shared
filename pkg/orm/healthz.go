package orm

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"gorm.io/gorm"
)

const (
	HealthURIKey   = "healthz"
	HealthURIValue = "/healthz"
)

type HealthController struct {
	db *gorm.DB
}

func (h *HealthController) Ping(c *gin.Context) {

	statusCode := 200
	statusMessage := "OK"

	db, err := h.db.DB()
	if err != nil {
		statusCode = 500
		statusMessage = fmt.Sprintf("connection to db failed. %v", err)
	}

	err = db.Ping()
	if err != nil {
		statusCode = 500
		statusMessage = fmt.Sprintf("ping test failed. %v", err)
	}

	c.JSON(statusCode, gin.H{"status": statusMessage, "appName": core.AppName, "version": core.Version})

}

func init() {
	ginshared.GetContainer().Provide(func(db *gorm.DB, route *gin.Engine) ginshared.DiController {
		controller := &HealthController{
			db: db,
		}

		baseUrl := ginshared.GetbaseUrl()
		viper.SetDefault(HealthURIKey, HealthURIValue)
		uri := viper.GetString(HealthURIKey)

		rootURI := HealthURIValue
		route.GET(rootURI, controller.Ping)
		if baseUrl != "" {
			route.GET(baseUrl+rootURI, controller.Ping)
		}
		if uri != "" && uri != rootURI {
			route.GET(uri, controller.Ping)
			if baseUrl != "" && uri[0] == '/' && len(baseUrl) > 0 && !(len(uri) >= len(baseUrl) && uri[:len(baseUrl)] == baseUrl) {
				route.GET(baseUrl+uri, controller.Ping)
			}
		}
		return controller
	}, ginshared.ControllerOptions)
}
