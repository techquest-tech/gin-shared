package tracing

import (
	"bytes"
	"io/ioutil"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type FullRequestDetails struct {
	gorm.Model
	Uri    string
	Body   string
	Status int
}

type TracingRequestService struct {
	DB     *gorm.DB
	Logger *zap.Logger
}

func NewTracingRequestService(db *gorm.DB, logger *zap.Logger) (*TracingRequestService, error) {
	tr := &TracingRequestService{
		DB:     db,
		Logger: logger,
	}
	if viper.GetBool(orm.KeyInitDB) {
		err := db.AutoMigrate(&FullRequestDetails{})
		if err != nil {
			logger.Error("create fullRequestDetals failed.", zap.Error(err))
		} else {
			logger.Info("create fullRequestDetails table done")
		}
	}
	return tr, nil
}

func (tr *TracingRequestService) LogfullRequestDetails(c *gin.Context) {
	data := make([]byte, 0)

	if c.Request.Body != nil {
		data, _ = ioutil.ReadAll(c.Request.Body)
		c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	}
	uri := c.Request.RequestURI

	c.Next()
	status := c.Writer.Status()
	go tr.doLogRequestBody(data, uri, status)
}

func (tr *TracingRequestService) doLogRequestBody(data []byte, uri string, status int) {
	req := FullRequestDetails{
		Body:   string(data),
		Status: status,
		Uri:    uri,
	}
	err := tr.DB.Save(&req).Error
	if err != nil {
		tr.Logger.Error("save reqest failed", zap.Error(err))
		return
	}
	tr.Logger.Info("save request details done.")
}
