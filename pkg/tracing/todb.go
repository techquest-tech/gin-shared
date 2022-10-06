package tracing

import (
	"github.com/asaskevich/EventBus"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type FullRequestDetails struct {
	gorm.Model
	TracingDetails
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
	if viper.GetBool("database.initDB") {
		err := db.AutoMigrate(&FullRequestDetails{})
		if err != nil {
			logger.Error("create fullRequestDetals failed.", zap.Error(err))
		} else {
			logger.Info("create fullRequestDetails table done")
		}
	}
	return tr, nil
}

func SubEventToDB(tr *TracingRequestServiceDBImpl, bus EventBus.Bus) {
	bus.SubscribeAsync(event.EventTracing, tr.doLogRequestBody, false)
}

func (tr *TracingRequestServiceDBImpl) doLogRequestBody(req *TracingDetails) {
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
