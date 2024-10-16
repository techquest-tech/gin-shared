package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var DefaultGormToipc = "scm.gorm.saved"

var GormMessagingEnabled = true

var ms MessagingService

func init() {
	core.ProvideStartup(func(m MessagingService) core.Startup {
		ms = m
		return nil
	})
}

func NewGormObjSyncService(ms MessagingService, logger *zap.Logger, db *gorm.DB) *GormObjSyncService {
	return &GormObjSyncService{
		MessageService: ms,
		DB:             db,
		Logger:         logger,
	}
}

type Sharding func(tx *gorm.DB, payload any) (tablename string, err error)

type GormObjSyncService struct {
	MessageService MessagingService
	DB             *gorm.DB
	Logger         *zap.Logger
	Sharding       Sharding
}

var cfg = &gorm.Session{
	SkipHooks:              true,
	NewDB:                  true,
	SkipDefaultTransaction: true,
}

func (ss *GormObjSyncService) ReceiveGormObjectSaved(ctx context.Context, topic, consumer string, raw []byte) error {
	kp, err := toKeyAndPayload(raw)
	if err != nil {
		ss.Logger.Error("unexpected payload", zap.Error(err))
		return err
	}
	tt, ok := m[kp.Key]
	if !ok {
		ss.Logger.Error("received object failed. unknown key, just drop it.", zap.String("key", kp.Key))
		return errors.New("received object failed. unknown key " + kp.Key)
	}
	payload := reflect.New(tt).Interface()
	err = json.Unmarshal(kp.Payload, payload)
	if err != nil {
		ss.Logger.Info("unexpected payload,", zap.Error(err))
		return err
	}
	tx := ss.DB.Session(cfg)

	if ss.Sharding != nil {
		tablename, err := ss.Sharding(tx, payload)
		if err != nil {
			return err
		}
		tx = tx.Table(tablename)
		ss.Logger.Info("sharding table for payload", zap.String("table", tablename))
	}

	switch kp.Action {
	case GormActionSave, "":
		err = tx.Save(payload).Error
		if err != nil {
			ss.Logger.Info("save object failed.", zap.Error(err), zap.String("data", tt.Name()), zap.Any("payload", payload))
			return err
		}
		ss.Logger.Info("save object done.", zap.String("data", tt.Name()))
	case GormActionDelete:
		err = tx.Delete(payload).Error
		if err != nil {
			ss.Logger.Error("delete object failed.", zap.Error(err), zap.String("data", tt.Name()), zap.Any("payload", payload))

			if tt, ok := payload.(IDbase); ok {
				id := tt.GetID()
				if id == 0 {
					ss.Logger.Warn("empty entity, just skip")
					return nil
				}
			}

			return err
		}
		ss.Logger.Info("delete object done.", zap.String("data", tt.Name()))
	default:
		ss.Logger.Info("unknown action.", zap.String("action", string(kp.Action)))
		return errors.ErrUnsupported
	}

	return nil
}

type GormAction string

const (
	GormActionSave   GormAction = "save"
	GormActionDelete GormAction = "delete"
)

type GormPayload struct {
	Key     string
	Action  GormAction
	Payload []byte
}

func toKeyAndPayload(raw []byte) (*GormPayload, error) {
	var payload GormPayload
	err := json.Unmarshal(raw, &payload)
	if err != nil {
		return nil, err
	}
	return &payload, nil
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

type IDbase interface {
	GetID() uint
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

	ms.Pub(ctx, DefaultGormToipc, GormPayload{Key: key, Payload: raw, Action: action})
	return nil
}

var SyncPageSize = 1000

// func ToSnake(raw string) string {
// 	parts := strings.Split(raw, "_")
// 	result := ""
// 	for _, item := range parts {
// 		if item == "id" {
// 			result = result + "ID"
// 		} else {
// 			result = result + strings.ToUpper(item[:1]) + item[1:]
// 		}

// 	}
// 	return result
// }
// func ToSnakeMap(raw map[string]any) map[string]any {
// 	result := make(map[string]any)
// 	for k, v := range raw {
// 		result[ToSnake(k)] = v
// 	}
// 	return result
// }

func PubEntitiesSince(ctx context.Context, key string, since time.Time) error {
	return core.GetContainer().Invoke(func(db *gorm.DB, logger *zap.Logger, msService MessagingService) error {
		if ms == nil {
			ms = msService
		}
		l := logger.With(zap.String("key", key))
		tt, ok := m[key]
		if !ok {
			keys := lo.Keys(m)
			return fmt.Errorf("%s is not registered, avaible keys %v", key, keys)
		}
		payload := reflect.New(tt).Interface()

		index := 0

		for {
			tx := db.WithContext(ctx).Model(payload).Order("updated_at").Limit(SyncPageSize).Offset(index * SyncPageSize)

			if !since.IsZero() {
				tx = tx.Where("updated_at > ?", since)
			}

			rr := make([]any, SyncPageSize)
			for i := 0; i < SyncPageSize; i++ {
				rr[i] = reflect.New(tt).Interface()
			}

			// rr := make([]map[string]any, 0)
			if err := tx.Find(&rr).Error; err != nil {
				return err
			}
			if len(rr) == 0 {
				l.Info("no more data")
				break
			}
			// l.Info("result len", zap.Int("len", len(rr)))
			// for _, m := range rr {
			// 	item := reflect.New(tt).Interface()
			// 	snakem := ToSnakeMap(m)
			// 	err := mapstructure.Decode(snakem, item)
			// 	if err != nil {
			// 		l.Error("decode failed", zap.Error(err))
			// 		return err
			// 	}
			// 	PubGormSaved(ctx, item)
			// }

			index++
		}

		return nil
	})
}
