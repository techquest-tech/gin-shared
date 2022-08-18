package event

import (
	"github.com/asaskevich/EventBus"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

var Bus EventBus.Bus

const (
	EventError    = "event.error"
	EventTracing  = "event.tracing"
	EventInit     = "event.gin.inited"
	EventStopping = "event.gin.stopping"
)

func init() {
	core.GetContainer().Provide(func(logger *zap.Logger) EventBus.Bus {
		logger.Info("event bus inited. use EventBus in memory")
		Bus = EventBus.New()
		return Bus
	})
}
