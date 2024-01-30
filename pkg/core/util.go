package core

import (
	"os"
	"sync"
	"time"

	"github.com/asaskevich/EventBus"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var startedEvent = sync.Once{}

var endEvent = sync.Once{}

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
				zap.L().Info("service started.")
			})
		}
	})
}

func NotifyStopping() {
	GetContainer().Invoke(func(p OptionalParam[EventBus.Bus]) {
		if p.P != nil {
			endEvent.Do(func() {
				p.P.Publish(EventStopping)
				p.P.WaitAsync()
				// time.Sleep(time.Second)
				zap.L().Info("service stopped")
			})
		}
	})

}

type ServiceParam struct {
	dig.In
	DB     *gorm.DB
	Logger *zap.Logger
	Bus    EventBus.Bus
}
