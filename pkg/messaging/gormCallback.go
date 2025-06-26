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

var (
	chSender            chan any
	ms                  MessagingService
	AbandonedChan       chan any
	GormCallbackEnabled = false
)

func init() {

	AbandonedChan = make(chan any)
	go core.AppendToFile(AbandonedChan, "receivedAbandoned.log")

	if GormCallbackEnabled {
		core.ProvideStartup(func(service MessagingService, db *gorm.DB) core.Startup {
			ms = service
			// if GormCallbackEnabled {
			db.Callback().Create().After("gorm:after_create").Register("messaging", messageCallbackForUpdate)
			db.Callback().Update().After("gorm:after_update").Register("messaging", messageCallbackForUpdate)
			db.Callback().Delete().After("gorm:delete").Register("messaging", messageCallbackForDelete)
			// } else {
			// zap.L().Info("gorm callback is disabled.")
			// }

			if GormCallbackEnabled || GormMessagingEnabled {
				chSender = make(chan any, 1000)
				go core.AppendToFile(chSender, "gormSenderAbandoned.log")
			}
			return nil
		})
	} else {
		zap.L().Info("gorm callback is disabled.")
	}

}

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
	if !GormCallbackEnabled {
		return nil
	}

	tt := reflect.TypeOf(payload)
	key := tt.String()
	key = strings.TrimLeft(key, "*")

	if _, ok := m[key]; !ok {
		zap.L().Info("not registered key, ignored.", zap.String("key", key))
		return nil
	}

	logger := zap.L().With(zap.String("topic", DefaultGormToipc), zap.Any("callback", string(action)), zap.String("key", key))

	id, hasID := GetPayloadID(payload)
	if hasID && id == 0 {
		logger.Warn("empty ID, just skip")
		chSender <- map[string]any{"key": key,
			"action":    string(action),
			"payload":   payload,
			"errorCode": "EmptyID"}
		return nil
	}

	logger.Debug("running callback.", zap.Any("payload", payload))
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Error("marshal payload failed.", zap.Error(err))
		return err
	}
	ms.Pub(ctx, DefaultGormToipc, GormPayload{Key: key, Payload: string(raw), Action: action})
	logger.Debug("callback done.")
	return nil
}

func messageCallbackForUpdate(db *gorm.DB) {
	if db.Error != nil || db.Statement.SkipHooks {
		return
	}
	if db.Statement.RowsAffected == 0 {
		zap.L().Debug("no rows affected, ignored.")
		return
	}
	payload := db.Statement.ReflectValue.Interface()
	pubGormAction(db.Statement.Context, payload, GormActionSave)
}

func messageCallbackForDelete(db *gorm.DB) {
	if db.Error != nil || db.Statement.SkipHooks {
		return
	}

	if db.Statement.RowsAffected == 0 {
		zap.L().Debug("no rows affected, ignored.")
		return
	}

	payload := db.Statement.ReflectValue.Interface()
	pubGormAction(db.Statement.Context, payload, GormActionDelete)
}
