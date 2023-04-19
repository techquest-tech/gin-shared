package ginshared

import (
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

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

var ComponentsOptions = dig.Group("components")

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

// var c = make([]Component, 0)

func RegisterComponent(comp Component) {
	GetContainer().Provide(func(logger *zap.Logger) Component {
		logger.Info("registed component", zap.String("component", fmt.Sprintf("%T", comp)))
		return comp
	}, ComponentsOptions)
}

type ParamComponents struct {
	dig.In
	Components []Component `group:"components"`
}

func init() {
	GetContainer().Provide(func(logger *zap.Logger, p ParamComponents) *Components {
		return &Components{
			Components: p.Components,
		}
	})
}
