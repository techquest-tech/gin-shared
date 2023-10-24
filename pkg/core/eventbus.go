package core

import (
	"github.com/asaskevich/EventBus"
	"go.uber.org/zap"
)

var Bus EventBus.Bus

const (
	EventError    = "event.error"
	EventTracing  = "event.tracing"
	EventInit     = "event.gin.inited"
	EventStopping = "event.gin.stopping"
	EventStarted  = "sys.started"
)

func init() {
	GetContainer().Provide(func(logger *zap.Logger) EventBus.Bus {
		logger.Info("event bus inited. use EventBus in memory")
		Bus = EventBus.New()
		return Bus
	})

}
