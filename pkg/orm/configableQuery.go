package orm

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/auth"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RawQuerySerice struct {
	db          *gorm.DB
	logger      *zap.Logger
	EnabledAuth bool
	Base        string
	Items       []RawQuery
}
type RawQuery struct {
	Uri    string
	Sql    string
	Preset map[string]interface{}
	Params []string
}

func init() {
	ginshared.GetContainer().Provide(initRawQuery, ginshared.ControllerOptions)
}

func initRawQuery(db *gorm.DB, logger *zap.Logger, router *gin.Engine, authservice *auth.AuthService) ginshared.DiController {
	settings := viper.Sub("Queries")
	if settings == nil {
		return nil
	}
	serivce := &RawQuerySerice{
		db:          db,
		logger:      logger,
		EnabledAuth: true,
	}

	settings.Unmarshal(serivce)

	logger.Debug("load query defines done", zap.Any("service", serivce))

	uri := viper.GetString("baseUri")
	serivce.Base = uri + serivce.Base

	// if serivce.EnabledAuth {
	group := auth.NewAuthedRouter(router, authservice, logger, serivce.Base, 299)
	// }
	if !serivce.EnabledAuth {
		logger.Warn("auth disabled for raw query", zap.String("base", serivce.Base))
		group = router.Group(serivce.Base)
	}

	for _, item := range serivce.Items {
		group.GET(item.Uri, serivce.handler(item))
	}

	return nil
}

func (service *RawQuerySerice) handler(item RawQuery) gin.HandlerFunc {
	return func(c *gin.Context) {
		allParams := map[string]interface{}{}

		for k, v := range item.Preset {
			allParams[k] = v
		}

		for _, v := range c.Params {
			allParams[v.Key] = v.Value
		}
		for k, v := range c.Request.URL.Query() {
			if len(v) == 1 {
				allParams[k] = v[0]
			} else {
				allParams[k] = v
			}

		}
		params := make([]interface{}, 0)
		for _, key := range item.Params {
			params = append(params, allParams[key])
		}
		result := make([]map[string]interface{}, 0)
		err := service.db.Raw(item.Sql, params...).Find(&result).Error

		if err != nil {
			panic(err)
		}

		c.JSON(http.StatusOK, result)
	}
}
