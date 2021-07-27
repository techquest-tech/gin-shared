package orm

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"github.com/spf13/viper"
	ginshared "github.com/techquest-tech/gin-shared/pkg/gin"
)

const (
	HealthURIKey   = "healthz"
	HealthURIValue = "/healthz"
)

type HealthController struct {
	db *gorm.DB
}

func (h *HealthController) Ping(c *gin.Context) {
	err := h.db.DB().Ping()
	statusCode := 200
	statusMessage := "OK"
	if err != nil {
		statusCode = 500
		statusMessage = fmt.Sprintf("connection to db failed. %v", err)
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
