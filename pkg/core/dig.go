package core

import (
	"go.uber.org/dig"
)

var Container = dig.New()

func Provide(constructor ...interface{}) {
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
