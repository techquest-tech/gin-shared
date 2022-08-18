package event

import (
	"github.com/asaskevich/EventBus"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

var Bus EventBus.Bus

const (
	EventError    = "event.error"
	EventTracing  = "event.tracing"
	EventInit     = "event.gin.inited"
	EventStopping = "event.gin.stopping"
	// EventPreStart    = "event.gin.beforestart"
	// EventPostStarted = "event.gin.started"
)

type EventComponent interface{}

type EventInited interface{}

type EventComponents struct {
	dig.In
	Componets []EventComponent `group:"events"`
}

var EventOptions = dig.Group("events")

func init() {
	core.GetContainer().Provide(func(logger *zap.Logger) EventBus.Bus {
		logger.Info("event bus inited. use EventBus in memory")
		Bus = EventBus.New()
		return Bus
	})
	core.GetContainer().Provide(func(p EventComponents) EventInited {
		zap.L().Info("event init done.")
		return "event inited done"
	})
}
