package tracing

import (
	"github.com/asaskevich/EventBus"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type FullRequestDetails struct {
	gorm.Model
	ginshared.TracingDetails
}

type TracingRequestServiceDBImpl struct {
	DB     *gorm.DB
	Logger *zap.Logger
}

func NewTracingRequestService(db *gorm.DB, logger *zap.Logger) (*TracingRequestServiceDBImpl, error) {
	tr := &TracingRequestServiceDBImpl{
		DB:     db,
		Logger: logger,
	}
	if viper.GetBool(orm.KeyInitDB) {
		err := db.AutoMigrate(&ginshared.TracingDetails{})
		if err != nil {
			logger.Error("create fullRequestDetals failed.", zap.Error(err))
		} else {
			logger.Info("create fullRequestDetails table done")
		}
	}
	return tr, nil
}

func SubEventToDB(tr *TracingRequestServiceDBImpl, bus EventBus.Bus) ginshared.DiController {
	bus.SubscribeAsync(event.EventTracing, tr.doLogRequestBody, false)
	return nil
}

func (tr *TracingRequestServiceDBImpl) doLogRequestBody(req *ginshared.TracingDetails) {
	model := FullRequestDetails{
		TracingDetails: *req,
	}
	err := tr.DB.Save(&model).Error
	if err != nil {
		tr.Logger.Error("save reqest failed", zap.Error(err))
		return
	}
	tr.Logger.Info("save request details done.", zap.Uint("targetID", req.TargetID))
}
