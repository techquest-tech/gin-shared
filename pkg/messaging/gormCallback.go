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
		db.Callback().Create().Register("messaging", messageCallbackForUpdate)
		db.Callback().Update().Register("messaging", messageCallbackForUpdate)
		db.Callback().Delete().Register("messaging", messageCallbackForDelete)
		return nil
	})
}

func PubGormSaved(ctx context.Context, payload any) error {
	return pubGormAction(ctx, payload, GormActionSave)
}

func PubGormDeleted(ctx context.Context, payload any) error {
	if tt, ok := payload.(IDbase); ok {
		if tt.GetID() == 0 {
			zap.L().Warn("empty entity, just skip")
			return nil
		}
	}
	pubGormAction(ctx, payload, GormActionDelete)
	return nil
}

func pubGormAction(ctx context.Context, payload any, action GormAction) error {
	if !GormMessagingEnabled {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tt := reflect.TypeOf(payload)
	key := tt.String()
	key = strings.TrimLeft(key, "*")

	if _, ok := m[key]; !ok {
		zap.L().Debug("not registered key, ignored.", zap.String("key", key))
		return nil
	}

	ms.Pub(ctx, DefaultGormToipc, GormPayload{Key: key, Payload: raw, Action: action})
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
