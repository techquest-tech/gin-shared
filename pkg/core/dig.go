package core

import "go.uber.org/dig"

var Container = dig.New()

func GetContainer() *dig.Container {
	return Container
}
