package tracing

import (
	"github.com/asaskevich/EventBus"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func NewConsoleTracing(bus EventBus.Bus, log *zap.Logger) ginshared.DiController {
	bus.SubscribeAsync(event.EventTracing, func(tracing *TracingDetails) {
		log.Info("tracing", zap.Any("details", tracing))
	}, false)
	return nil
}
