package query

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/auth"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RawQuerySerice struct {
	db          *gorm.DB
	logger      *zap.Logger
	Source      string
	EnabledAuth bool
	Base        string
	Items       []SerivceItem
}

type SerivceItem struct {
	Uri     string
	Query   RawQuery
	Details *RawQuery
}

func init() {
	ginshared.GetContainer().Provide(initRawQuery, ginshared.ControllerOptions)
}

func initRawQuery(logger *zap.Logger, router *gin.Engine, authservice *auth.AuthService, db *gorm.DB) ginshared.DiController {
	settings := viper.Sub("Queries")
	if settings == nil {
		logger.Warn("not queries in config files, ignored.")
		return nil
	}
	serivce := &RawQuerySerice{
		logger:      logger,
		EnabledAuth: true,
	}

	settings.Unmarshal(serivce)

	//init DB connections.
	dbsettings := serivce.Source
	serivce.db = orm.InitDB(dbsettings, logger)

	if serivce.db == nil {
		serivce.db = db
	}

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
		if item.Details == nil {
			group.GET(item.Uri, serivce.handler(item.Query))
		} else {
			group.GET(item.Uri, serivce.handleDetails(item.Query, *item.Details))
		}

	}

	return nil
}

func readParams(c *gin.Context) map[string]interface{} {
	allParams := map[string]interface{}{}

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
	return allParams
}

func (service *RawQuerySerice) handleDetails(header, details RawQuery) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := readParams(c)
		result := map[string]interface{}{}
		r, err := header.Query(service.db, p)
		if err != nil {
			service.logger.Error("read header information failed.", zap.Error(err), zap.String("sql", header.Sql))
			panic(err)
		}
		switch len(r) {
		case 1:
			result = r[0]
			for k, v := range r[0] {
				p[k] = v
			}
		case 0:
			service.logger.Warn("read header failed, no records found", zap.String("sql", header.Sql))
			c.JSON(404, "no records found")
			return
		default:
			service.logger.Warn("read header failed, multi records found.", zap.String("sql", header.Sql), zap.Int("len", len(r)))
			panic(fmt.Errorf("multi-recourds found"))
		}

		//read details
		sub, err := details.Query(service.db, p)
		if err != nil {
			service.logger.Error("read detail records failed.", zap.Error(err), zap.String("sql", details.Sql))
			panic(err)
		}
		result["details"] = sub
		c.JSON(http.StatusOK, result)
	}
}

func (service *RawQuerySerice) handler(item RawQuery) gin.HandlerFunc {
	return func(c *gin.Context) {
		allParams := readParams(c)
		result, err := item.Query(service.db, allParams)

		if err != nil {
			panic(err)
		}

		c.JSON(http.StatusOK, result)
	}
}
