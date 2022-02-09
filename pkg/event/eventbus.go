package event

import (
	"github.com/asaskevich/EventBus"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func init() {
	ginshared.GetContainer().Provide(func(logger *zap.Logger) EventBus.Bus {
		logger.Info("event bus inited. use EventBus in memory")
		return EventBus.New()
	})
}
