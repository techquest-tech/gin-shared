package core

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	startedEvent  = sync.Once{}
	beforebootup  = sync.Once{}
	delay         time.Duration
	once          sync.Once
	GraceShutdown = 5 * time.Second
)

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

func NotifyStopping() { // not used anymore, empty fun only.
}

var EnvValues []string

func BeforeBootup(key string) {
	beforebootup.Do(func() {
		// load .env if file exists
		godotenv.Load("config/.env", ".env")
		// if err != nil {
		// 	fmt.Println("read .env file failed. ignored.", err.Error())
		// }

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

func CloseOnlyNotified() {
	once.Do(func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		signal.Notify(sigCh, syscall.SIGTERM)

		<-sigCh

		logger := zap.L()

		fmt.Println("app existing...")

		ctx, c := context.WithTimeout(context.Background(), GraceShutdown)

		defer c()

		shutdownDone := make(chan bool)

		go func() {
			Bus.Publish(EventStopping)
			Bus.WaitAsync()
			shutdownDone <- true
		}()

		select {
		case <-ctx.Done():
			logger.Warn("graceful shutdown timed out, forcing shutdown")
		case <-shutdownDone:
			// fmt.Printf("done\n")
			logger.Info("cleanup done.")
		}

		if delay > 0 {
			logger.Info("delaying shutdown for", zap.Duration("duration", delay))
			time.Sleep(delay)
		}

		logger.Info("service stopped")
	})
}

func PrintVersion() {
	content, err := os.ReadFile("version.txt")
	if err == nil {
		println(string(content))
	}

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

func ToAnyChan[T any](input chan T) chan any {
	output := make(chan any)
	go func() {
		for val := range input {
			output <- val
		}
	}()
	OnServiceStopping(func() {
		close(output)
	})
	return output
}

func GetStructNameOnly[T any](rr T) string {
	// get the struct name without package

	tname := fmt.Sprintf("%T", rr)

	from := strings.LastIndexByte(tname, '.')
	from2 := strings.LastIndexByte(tname, '[')

	if from < from2 {
		from = from2
	}

	if from > 0 {
		from = from + 1
	}
	to := strings.LastIndexByte(tname, ']')
	if to == -1 {
		to = len(tname)
	}

	return tname[from:to]
}

func ReplaceTablePrefix(raw string, prefixes ...string) string {
	prefix := viper.GetString("database.tablePrefix")
	if len(prefixes) > 0 {
		prefix = prefixes[0]
	}
	return strings.ReplaceAll(raw, "{{.tableprefix}}", prefix)
}

func IsStructOrPtrToStruct(v interface{}) bool {
	val := reflect.ValueOf(v)
	kind := val.Kind()
	return kind == reflect.Struct || (kind == reflect.Ptr && val.Elem().Kind() == reflect.Struct)
}
