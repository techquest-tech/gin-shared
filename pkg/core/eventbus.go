package core

import (
	"github.com/asaskevich/EventBus"
	"go.uber.org/zap"
)

var Bus EventBus.Bus

const (
	EventError    = "event.error"
	EventTracing  = "event.tracing"
	EventInit     = "event.gin.inited" //trigger when gin ready to service.
	EventStopping = "event.gin.stopping"
	EventStarted  = "sys.started" //trigger when all inited done.
)

func init() {
	GetContainer().Provide(func(logger *zap.Logger) EventBus.Bus {
		logger.Info("event bus inited. use EventBus in memory")
		Bus = EventBus.New()
		return Bus
	})
}

// type SystenEvent func()

// func regEvent(e string, fn SystenEvent) {
// 	ProvideStartup(func(bus EventBus.Bus) Startup {
// 		bus.SubscribeAsync(e, fn, false)
// 		return nil
// 	})
// }

// func OnServiceStarted(fn SystenEvent) {
// 	regEvent(EventStarted, fn)
// }

// func OnServiceStopping(fn SystenEvent) {
// 	regEvent(EventStopping, fn)
// }
