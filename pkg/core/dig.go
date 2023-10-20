package core

import (
	"log"
	"os"
	"sync"

	"go.uber.org/dig"
)

var Container = dig.New()

const (
	NO_INIT = "SCM_MUTED"
)

var cc = sync.Once{}

func Ignored() bool {
	noinit := os.Getenv(NO_INIT)
	return noinit == "true"
}

func Provide(constructor ...interface{}) {
	if Ignored() {
		cc.Do(func() {
			log.Println("Init is disabled.")
		})
		return
	}

	cc.Do(func() {
		log.Println("Init is enabled.")
	})

	for _, item := range constructor {
		GetContainer().Provide(item)
	}
}

func GetContainer() *dig.Container {
	return Container
}

type Startup interface{}

var StartupOptions = dig.Group("startups")

func ProvideStartup(constructor ...any) {
	for _, item := range constructor {
		GetContainer().Provide(item, StartupOptions)
	}
}

type OptionalParam[T any] struct {
	dig.In
	P T `optional:"true"`
}
