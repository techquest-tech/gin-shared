package core

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/asaskevich/EventBus"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var startedEvent = sync.Once{}

// var endEvent = sync.Once{}

var delay time.Duration

func NotifyStarted() {
	go GetContainer().Invoke(func(p OptionalParam[EventBus.Bus]) {
		if p.P != nil {
			startedEvent.Do(func() {
				dur := os.Getenv("SCM_DUR_STARTED")
				if dur == "" {
					dur = "200ms"
				}
				d, err := time.ParseDuration(dur)
				if err != nil {
					return
				}

				delay = d
				time.Sleep(d)
				p.P.Publish(EventStarted)
				zap.L().Info("service started.")
			})
		}
	})
}

func NotifyStopping() {
	// GetContainer().Invoke(func(p OptionalParam[EventBus.Bus]) {
	// 	if p.P != nil {
	// 		endEvent.Do(func() {
	// 			p.P.Publish(EventStopping)
	// 			p.P.WaitAsync()
	// 			// time.Sleep(time.Second)
	// 			zap.L().Info("service stopped")
	// 		})
	// 	}
	// })

}

type ServiceParam struct {
	dig.In
	DB     *gorm.DB
	Logger *zap.Logger
	Bus    EventBus.Bus
}

var once sync.Once

func CloseOnlyNotified() {
	once.Do(func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		signal.Notify(sigCh, syscall.SIGTERM)

		<-sigCh
		context.TODO().Done()

		fmt.Printf("app existing...")

		Bus.Publish(EventStopping)
		Bus.WaitAsync()

		if delay > 0 {
			time.Sleep(delay)
		}

		zap.L().Info("service stopped")
	})
}

func PrintVersion() {
	zap.L().Info("Application info:", zap.String("appName", AppName),
		zap.String("verion", Version),
		zap.String("Go version", runtime.Version()),
	)
}
