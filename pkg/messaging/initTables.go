package messaging

import (
	"reflect"
	"strings"

	"go.uber.org/zap"
)

var m = map[string]reflect.Type{}
var revert = map[reflect.Type]string{}

var registedEntity = []any{}

func Reg(payload any) {
	tt := reflect.TypeOf(payload)
	// if key == "" {
	key := tt.String()
	key = strings.TrimLeft(key, "*")
	// }
	m[key] = tt
	revert[tt] = key

	zap.L().Info("gorm object registered.", zap.String("key", key), zap.String("type", tt.String()))

	registedEntity = append(registedEntity, payload)
}

// GetRegisted function is used to obtain a list of registered entities.
func GetRegisted() []any {
	return registedEntity
}
