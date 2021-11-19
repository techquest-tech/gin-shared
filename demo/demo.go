package main

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/tbaehler/gin-keycloak/pkg/ginkeycloak"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
)

type DemoController struct {
}

// func (d *DemoController) GetControllerName() string {
// 	return "helloWorld"
// }

func (d *DemoController) Hello(c *gin.Context) {
	c.JSON(200, gin.H{"hello": "world"})
}

func (d *DemoController) Admin(c *gin.Context) {
	c.JSON(200, gin.H{"hello": "Admin"})
}

// NewDemoController must return ginshared.DiController --- Step 1
func NewDemoController(router *gin.Engine, logger *zap.Logger) ginshared.DiController {

	var sbbEndpoint = ginkeycloak.KeycloakConfig{
		Url:   "https://sso.sit-k8s.esquel.cn/",
		Realm: "rfid",
	}

	controller := &DemoController{}
	router.Use(ginkeycloak.Auth(ginkeycloak.AuthCheck(), sbbEndpoint)).GET("/healthz", controller.Hello)
	logger.Info("controller is ready.")

	config := ginkeycloak.BuilderConfig{
		Service: "hazzys",
		Url:     "https://sso.sit-k8s.esquel.cn/",
		Realm:   "rfid",
	}

	auth := ginkeycloak.NewAccessBuilder(config).RestrictButForRole("admin").Build()

	router.Group("/admin").Use(auth).GET("/hello", controller.Admin)

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
