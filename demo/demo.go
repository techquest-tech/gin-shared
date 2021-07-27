package main

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	ginshared "github.com/techquest-tech/gin-shared/pkg/gin"
)

type DemoController struct {
}

// func (d *DemoController) GetControllerName() string {
// 	return "helloWorld"
// }

func (d *DemoController) Hello(c *gin.Context) {
	c.JSON(200, gin.H{"hello": "world"})
}

// NewDemoController must return ginshared.DiController --- Step 1
func NewDemoController(router *gin.Engine, logger *zap.Logger) ginshared.DiController {
	controller := &DemoController{}
	router.GET("/healthz", controller.Hello)
	logger.Info("controller is ready.")
	return controller
}

func main() {
	//ginshared.ControllerOptions is MUST！ --- Step 2
	ginshared.GetContainer().Provide(NewDemoController, ginshared.ControllerOptions)

	ginshared.Start()

	// ginshared.GetContainer().Invoke(func(p *ginshared.Params) error {
	// 	req, _ := http.NewRequest("GET", "/healthz", nil)
	// 	w := httptest.NewRecorder()
	// 	p.Router.ServeHTTP(w, req)

	// 	assert.Equal(t, http.StatusOK, w.Code)
	// 	return nil
	// })
}
