package core

import (
	"sort"

	"github.com/gin-gonic/gin"
	"go.uber.org/dig"
	"go.uber.org/zap"
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

type Component interface {
	Priority() int
	OnEngineInited(r *gin.Engine) error
}

type DefaultComponent struct{}

func (dc *DefaultComponent) Priority() int {
	return 0
}

type Components struct {
	Components []Component
}

func (cs *Components) InitAll(r *gin.Engine) {
	sort.Slice(cs.Components, func(i, j int) bool {
		return cs.Components[i].Priority() > cs.Components[j].Priority()
	})
	for _, item := range cs.Components {
		err := item.OnEngineInited(r)
		if err != nil {
			zap.L().Error("init component failed.", zap.Error(err))
			panic(err)
		}
	}
}

var c = make([]Component, 0)

func RegisterComponent(comp Component) {
	c = append(c, comp)
}

func init() {
	GetContainer().Provide(func(logger *zap.Logger) *Components {
		return &Components{
			Components: c,
		}
	})
}
