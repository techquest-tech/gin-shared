package orm

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/spf13/viper"
	ginshared "github.com/techquest-tech/gin-shared/pkg/gin"
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

	c.JSON(statusCode, gin.H{"status": statusMessage})

}

func init() {
	ginshared.GetContainer().Provide(func(db *gorm.DB, route *gin.Engine) ginshared.DiController {
		controller := &HealthController{
			db: db,
		}

		viper.SetDefault(HealthURIKey, HealthURIValue)
		uri := viper.GetString(HealthURIKey)

		route.GET(uri, controller.Ping)
		return controller
	}, ginshared.ControllerOptions)
}
