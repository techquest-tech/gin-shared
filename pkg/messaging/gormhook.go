package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

type IDbase interface {
	GetID() uint
}

const SyncPageSize = 1000

type QueryFn func(ctx context.Context, db *gorm.DB, logger *zap.Logger, since time.Time, index int) ([]any, error)

func QueryEntities[T any](ctx context.Context, db *gorm.DB, logger *zap.Logger, since time.Time, index int) ([]any, error) {
	tx := db.WithContext(ctx)

	if !since.IsZero() {
		tx = tx.Where("updated_at > ?", since)
	}
	tx = tx.Order("updated_at").Limit(SyncPageSize).Offset(index * SyncPageSize)

	rr := make([]T, 0)
	if err := tx.Find(&rr).Error; err != nil {
		logger.Error("query entities failed", zap.Error(err))
		return nil, err
	}
	return lo.ToAnySlice(rr), nil
}

func PubEntitiesSince(ctx context.Context, key string, since time.Time) error {
	return core.GetContainer().Invoke(func(db *gorm.DB, logger *zap.Logger, msService MessagingService) error {
		if ms == nil {
			ms = msService
		}
		l := logger.With(zap.String("key", key))
		fn, ok := mSlice[key]
		if !ok {
			keys := lo.Keys(m)
			return fmt.Errorf("%s is not registered, avaible keys %v", key, keys)
		}
		index := 0
		processed := 0

		for {
			rr, err := fn(ctx, db, l, since, index)
			if err != nil {
				return err
			}
			l.Info("result len", zap.Int("len", len(rr)))

			for _, item := range rr {
				PubGormSaved(ctx, item)
			}
			processed += len(rr)
			index++
			if len(rr) < SyncPageSize {
				l.Info("no more data", zap.Int("processed", processed))
				return nil
			}
		}
	})
}
