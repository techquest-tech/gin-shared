package swagger

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
)

type SwaggerController struct{}

func init() {
	ginshared.GetContainer().Provide(initSwaggerController, ginshared.ControllerOptions)
}

func initSwaggerController(router *gin.Engine) ginshared.DiController {
	if mode := gin.Mode(); mode == gin.DebugMode {
		url := ginSwagger.URL("/swagger/doc.json")
		router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, url))
	}
	return SwaggerController{}

}
