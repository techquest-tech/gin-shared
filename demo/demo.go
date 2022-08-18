package main

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/keycloak"
	_ "github.com/techquest-tech/gin-shared/pkg/orm"
	_ "github.com/techquest-tech/gin-shared/pkg/prom"
	_ "github.com/techquest-tech/gin-shared/pkg/tracing"
	_ "github.com/techquest-tech/gin-shared/pkg/tracing/azure"
)

type DemoController struct {
	logger *zap.Logger
}

// func (d *DemoController) GetControllerName() string {
// 	return "helloWorld"
// }

func (d *DemoController) Hello(c *gin.Context) {
	currently, ok := c.Get("token")
	if ok {
		d.logger.Info("user infor", zap.Any("current", currently))
	}

	c.JSON(200, gin.H{"hello": "world"})
}

func (d *DemoController) Admin(c *gin.Context) {

	c.JSON(200, gin.H{"hello": "Admin"})
}

// NewDemoController must return ginshared.DiController --- Step 1
func NewDemoController(router *gin.Engine, logger *zap.Logger, keycloak *keycloak.KeycloakConfig) ginshared.DiController {

	// var sbbEndpoint = ginkeycloak.KeycloakConfig{
	// 	Url:   "https://sso.sit-k8s.esquel.cn/",
	// 	Realm: "rfid",
	// }

	controller := &DemoController{
		logger: logger,
	}

	// router.Use(ginkeycloak.Auth(ginkeycloak.AuthCheck(), sbbEndpoint)).GET("/healthz", controller.Hello)
	// logger.Info("controller is ready.")

	// config := ginkeycloak.BuilderConfig{
	// 	Service: "hazzys",
	// 	Url:     "https://sso.sit-k8s.esquel.cn/",
	// 	Realm:   "rfid",
	// }

	// x := ginkeycloak.NewAccessBuilder(config)
	// auth := x.RestrictButForRealm("admin").Build()

	router.Group("/private").Use(keycloak.Auth()).GET("/hello", controller.Admin)
	router.Group("/api").Use(keycloak.Auth()).GET("/hello", controller.Hello)

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
