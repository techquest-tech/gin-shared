package core

import (
	"github.com/asaskevich/EventBus"
	"go.uber.org/zap"
)

var Bus = EventBus.New()

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
		return Bus
	})
}

type SystenEvent func()

// OnServiceStarted make sure call this func after
func OnServiceStarted(fn SystenEvent) {
	Bus.SubscribeAsync(EventStarted, fn, false)
}

func InvokeOnServiceStarted(fn any) {
	Bus.Subscribe(EventStarted, func() {
		GetContainer().Invoke(fn)
	})
}
func InvokeAsyncOnServiceStarted(fn any) {
	Bus.SubscribeAsync(EventStarted, func() {
		GetContainer().Invoke(fn)
	}, false)
}

func OnServiceStopping(fn SystenEvent) {
	Bus.SubscribeOnce(EventStopping, fn)
}

func OnEvent(topic string, fn SystenEvent) {
	Bus.Subscribe(topic, fn)
}
