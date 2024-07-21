package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var DefaultGormToipc = "scm.gorm.saved"

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
	SkipHooks: true,
	NewDB:     true,
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
	GormActionSave   = "save"
	GormActionDelete = "delete"
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
	return pubGormAction(ctx, payload, GormActionDelete)
}

type IDbase interface {
	GetID() uint
}

func pubGormAction(ctx context.Context, payload any, action GormAction) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tt := reflect.TypeOf(payload)
	key := tt.String()
	key = strings.TrimLeft(key, "*")

	return ms.Pub(ctx, DefaultGormToipc, GormPayload{Key: key, Payload: raw, Action: action})
}
