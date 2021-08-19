package ginshared

import (
	"go.uber.org/dig"
)

var container = dig.New()

func GetContainer() *dig.Container {
	return container
}

type DiController interface {
	// GetControllerName() string
}
