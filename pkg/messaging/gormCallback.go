package messaging

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func init() {
	core.ProvideStartup(func(ms MessagingService, db *gorm.DB) core.Startup {
		db.Callback().Create().After("gorm:after_create").Register("messaging", messageCallbackForUpdate)
		db.Callback().Update().After("gorm:after_update").Register("messaging", messageCallbackForUpdate)
		db.Callback().Delete().After("gorm:delete").Register("messaging", messageCallbackForDelete)
		return nil
	})
}

// func PubGormSaved(ctx context.Context, payload any) error {
// 	return pubGormAction(ctx, payload, GormActionSave)
// }

// func PubGormDeleted(ctx context.Context, payload any) error {
// 	pubGormAction(ctx, payload, GormActionDelete)
// 	return nil
// }

// try to get payload ID value as uint, return false if payload doesn't have ID field
func GetPayloadID(payload any) (uint, bool) {
	if payload == nil {
		return 0, false
	}
	vv := reflect.ValueOf(payload)
	if vv.Kind() == reflect.Ptr {
		vv = vv.Elem()
	}

	if vvv := vv.FieldByName("ID"); vvv.IsValid() {
		id := vvv.Interface().(uint)
		return id, true
	}

	return 0, false
}

func pubGormAction(ctx context.Context, payload any, action GormAction) error {
	if !GormMessagingEnabled {
		return nil
	}

	tt := reflect.TypeOf(payload)
	key := tt.String()
	key = strings.TrimLeft(key, "*")

	if _, ok := m[key]; !ok {
		zap.L().Debug("not registered key, ignored.", zap.String("key", key))
		return nil
	}

	logger := zap.L().With(zap.String("topic", DefaultGormToipc), zap.Any("callback", string(action)), zap.String("key", key))

	id, hasID := GetPayloadID(payload)
	if hasID && id == 0 {
		logger.Warn("empty ID, just skip")
		return nil
	}

	logger.Debug("running callback.", zap.Any("payload", payload))
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ms.Pub(ctx, DefaultGormToipc, GormPayload{Key: key, Payload: raw, Action: action})
	logger.Debug("callback done.")
	return nil
}

func messageCallbackForUpdate(db *gorm.DB) {
	if db.Error != nil || db.Statement.SkipHooks {
		return
	}
	payload := db.Statement.ReflectValue.Interface()
	pubGormAction(db.Statement.Context, payload, GormActionSave)
}

func messageCallbackForDelete(db *gorm.DB) {
	if db.Error != nil || db.Statement.SkipHooks {
		return
	}
	payload := db.Statement.ReflectValue.Interface()
	pubGormAction(db.Statement.Context, payload, GormActionDelete)
}
