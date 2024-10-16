package messaging

import (
	"reflect"
	"strings"

	"go.uber.org/zap"
)

var m = map[string]reflect.Type{}

var mSlice = map[string]QueryFn{}

var revert = map[reflect.Type]string{}

var registedEntity = []any{}

// func Reg(payload any) {
// 	tt := reflect.TypeOf(payload)
// 	// if key == "" {
// 	key := tt.String()
// 	key = strings.TrimLeft(key, "*")
// 	// }
// 	if _, ok := m[key]; ok {
// 		zap.L().Warn("registered duplicated, ignored.")
// 		return
// 	}
// 	m[key] = tt
// 	revert[tt] = key

// 	zap.L().Info("gorm object registered.", zap.String("key", key), zap.String("type", tt.String()))

// 	registedEntity = append(registedEntity, payload)
// }

// GetRegisted function is used to obtain a list of registered entities.
func GetRegisted() []any {
	return registedEntity
}

func Reg[T any](payload T) {
	tt := reflect.TypeOf(payload)
	// if key == "" {
	key := tt.String()
	key = strings.TrimLeft(key, "*")
	// }
	if _, ok := m[key]; ok {
		zap.L().Warn("registered duplicated, ignored.")
		return
	}
	m[key] = tt
	mSlice[key] = QueryEntities[T]

	revert[tt] = key

	zap.L().Info("gorm object registered.", zap.String("key", key), zap.String("type", tt.String()))

	registedEntity = append(registedEntity, payload)
}
