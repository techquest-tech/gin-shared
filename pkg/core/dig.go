package core

import (
	"os"

	"go.uber.org/dig"
)

var Container = dig.New()

const (
	NO_INIT = "_scm_no_init"
)

func Ignored() bool {
	noinit := os.Getenv(NO_INIT)
	return noinit == ""
}

func Provide(constructor ...interface{}) {
	if Ignored() {
		return
	}
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
