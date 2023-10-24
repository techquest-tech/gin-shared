package core

import (
	"sync"

	"github.com/asaskevich/EventBus"
	"go.uber.org/zap"
)

var startedEvent = sync.Once{}

func NotifyStarted() {
	GetContainer().Invoke(func(p OptionalParam[EventBus.Bus]) {
		if p.P != nil {
			startedEvent.Do(func() {
				p.P.Publish(EventStarted)
				zap.L().Info("app started.")
			})
		}
	})
}
