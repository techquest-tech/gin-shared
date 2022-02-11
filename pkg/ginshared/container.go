package ginshared

import (
	"go.uber.org/dig"
)

var container = dig.New()

func GetContainer() *dig.Container {
	return container
}

type DiController interface{}

var ControllerOptions = dig.Group("controllers")

func Provide(constructor interface{}, opts ...dig.ProvideOption) error {
	return container.Provide(constructor, opts...)
}

func ProvideController(constructor interface{}) error {
	return container.Provide(constructor, ControllerOptions)
}
