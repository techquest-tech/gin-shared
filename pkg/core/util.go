package core

import (
	"os"
	"sync"
	"time"

	"github.com/asaskevich/EventBus"
	"go.uber.org/zap"
)

var startedEvent = sync.Once{}

func NotifyStarted() {
	go GetContainer().Invoke(func(p OptionalParam[EventBus.Bus]) {
		if p.P != nil {
			startedEvent.Do(func() {
				dur := os.Getenv("SCM_DUR_STARTED")
				if dur == "" {
					dur = "2s"
				}
				d, err := time.ParseDuration(dur)
				if err != nil {
					return
				}
				time.Sleep(d)
				p.P.Publish(EventStarted)
				zap.L().Info("app started.")
			})
		}
	})
}
