package core

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"reflect"
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

var beforebootup = sync.Once{}

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

func BeforeBootup(key string) {
	beforebootup.Do(func() {
		Provide(func() ConfigSecret {
			return ConfigSecret(key)
		})
		InitEmbedConfig()
	})

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

func Clone(original any, target any) error {
	if reflect.TypeOf(target).Kind() != reflect.Ptr || reflect.TypeOf(original).Kind() != reflect.Ptr {
		return fmt.Errorf("original and target must be a pointer")
	}
	value := reflect.ValueOf(original).Elem()
	clone := reflect.ValueOf(target).Elem()

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		targetField := clone.Field(i)
		if targetField.IsZero() && !field.IsZero() {
			targetField.Set(field)
		}
	}

	return nil
}

type Md5Value interface {
	ToMd5() []byte
}

func ToMd5(items ...any) string {
	if len(items) == 0 {
		return ""
	}
	h := md5.New()
	for _, item := range items {
		if b, ok := item.([]byte); ok {
			h.Write(b)
		} else if s, ok := item.(string); ok {
			h.Write([]byte(s))
		} else if impled, ok := item.(Md5Value); ok {
			h.Write(impled.ToMd5())
		} else {
			raw, err := json.Marshal(item)
			if err != nil {
				zap.L().Error("json marshal for idempotent failed", zap.Error(err))
				panic(err)
			}
			h.Write(raw)
		}
	}
	signed := hex.EncodeToString(h.Sum(nil))
	return signed
}

func MD5(raw []byte) string {
	h := md5.New()
	h.Write(raw)
	signed := hex.EncodeToString(h.Sum(nil))
	return signed
}
