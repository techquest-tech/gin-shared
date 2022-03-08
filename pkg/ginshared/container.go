package ginshared

import (
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/dig"
)

// var container = dig.New()

func GetContainer() *dig.Container {
	return core.Container
}

type DiController interface{}

var ControllerOptions = dig.Group("controllers")

func Provide(constructor interface{}, opts ...dig.ProvideOption) error {
	return core.Container.Provide(constructor, opts...)
}

func ProvideController(constructor interface{}) error {
	return core.Container.Provide(constructor, ControllerOptions)
}
