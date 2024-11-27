package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var DefaultGormToipc = "scm.gorm.saved"

var GormMessagingEnabled = true

func NewGormObjSyncService(ms MessagingService, logger *zap.Logger, db *gorm.DB) *GormObjSyncService {
	return &GormObjSyncService{
		MessageService: ms,
		DB:             db,
		Logger:         logger,
	}
}

type Sharding func(tx *gorm.DB, key string, payload any) (tablename string, err error)

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

func (ss *GormObjSyncService) toAbandoned(kp *GormPayload, errCode, topic, consumer string) {
	AbandonedChan <- map[string]any{
		"error":    errCode,
		"topic":    topic,
		"consumer": consumer,
		"key":      kp.Key,
		"action":   kp.Action,
		"data":     kp.Payload,
	}
}

func (ss *GormObjSyncService) ReceiveGormObjectSaved(ctx context.Context, topic, consumer string, raw []byte) error {
	kp, err := toKeyAndPayload(raw)
	if err != nil {
		ss.Logger.Error("unexpected payload", zap.Error(err))
		return err
	}
	l := ss.Logger.With(zap.String("key", kp.Key))
	tt, ok := m[kp.Key]
	if !ok {
		l.Error("received object failed. unknown key, just drop it.", zap.String("key", kp.Key))
		ss.toAbandoned(kp, "UnknownKey", topic, consumer)
		return nil
	}
	payload := reflect.New(tt).Interface()
	err = json.Unmarshal(kp.Payload, payload)
	if err != nil {
		l.Info("unexpected payload,", zap.Error(err))
		ss.toAbandoned(kp, err.Error(), topic, consumer)
		return err
	}
	tx := ss.DB.Session(cfg)

	id, hasID := GetPayloadID(payload)

	if ss.Sharding != nil {
		tablename, err := ss.Sharding(tx, kp.Key, payload)
		if err != nil {
			ss.toAbandoned(kp, "ShardingFailed:"+err.Error(), topic, consumer)
			return err
		}
		tx = tx.Table(tablename)
		l.Debug("sharding table for payload", zap.String("table", tablename))
	}

	switch kp.Action {
	case GormActionSave, "":
		err = tx.Save(payload).Error
		if err != nil {
			l.Error("save object failed.", zap.Error(err), zap.String("data", tt.Name()), zap.Any("payload", payload))
			ss.toAbandoned(kp, "SaveFailed:"+err.Error(), topic, consumer)
			return err
		}
		l.Info("save object done.", zap.String("data", tt.Name()), zap.Uint("id", id))
	case GormActionDelete:
		if hasID && id == 0 {
			l.Warn("empty ID for delete action, just ignore it.", zap.Any("payload", payload))
			// DroppedPayload.Push(&FailedPayload{Payload: payload, Key: kp.Key, FailedCode: "empty_id"})
			ss.toAbandoned(kp, "DeletedOnEmptyID", topic, consumer)
			return nil
		}
		err = tx.Delete(payload).Error
		if err != nil {
			l.Error("delete object failed.", zap.Error(err), zap.String("data", tt.Name()), zap.Any("payload", payload))
			ss.toAbandoned(kp, "DeleteFailed:"+err.Error(), topic, consumer)
			return err
		}
		l.Info("delete object done.", zap.String("data", tt.Name()), zap.Uint("id", id))
	default:
		ss.Logger.Info("unknown action.", zap.String("action", string(kp.Action)))
		// return errors.ErrUnsupported
		ss.toAbandoned(kp, "UnknownAction", topic, consumer)
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

const SyncPageSize = 1000

type QueryFn func(ctx context.Context, db *gorm.DB, logger *zap.Logger, since time.Time, to time.Time, index int, queryDeleted bool) ([]any, error)

func QueryEntities[T any](ctx context.Context, db *gorm.DB, logger *zap.Logger, since time.Time, to time.Time, index int, queryDeleted bool) ([]any, error) {
	tx := db.WithContext(ctx)

	if queryDeleted {
		tx = tx.Unscoped()
		tx = tx.Where("deleted_at is not null")
		if !since.IsZero() {
			tx = tx.Where("deleted_at > ?", since)
		}
		if !to.IsZero() {
			tx = tx.Where("deleted_at <= ?", to)
		}
		tx = tx.Order("deleted_at")
	} else {
		if !since.IsZero() {
			tx = tx.Where("updated_at > ?", since)
		}
		if !to.IsZero() {
			tx = tx.Where("updated_at <= ?", to)
		}
		tx = tx.Order("updated_at")
	}

	tx = tx.Debug().Limit(SyncPageSize).Offset(index * SyncPageSize)

	rr := make([]T, 0)
	if err := tx.Find(&rr).Error; err != nil {
		logger.Error("query entities failed", zap.Error(err))
		return nil, err
	}
	return lo.ToAnySlice(rr), nil
}

func PubEntitiesSince(ctx context.Context, key string, since time.Time, to time.Time) error {
	return core.GetContainer().Invoke(func(db *gorm.DB, logger *zap.Logger, msService MessagingService) error {
		if ms == nil {
			ms = msService
		}
		l := logger.With(zap.String("key", key))
		fn, ok := mSlice[key]
		if !ok {
			keys := lo.Keys(m)
			sort.Strings(keys)
			return fmt.Errorf("%s is not registered, avaible keys: \n%s", key, strings.Join(keys, "\t\n"))
		}

		fnitem := func(deleted bool) error {
			index := 0
			processed := 0

			for {
				rr, err := fn(ctx, db, l, since, to, index, deleted)
				if err != nil {
					return err
				}
				l.Info("result len", zap.Int("len", len(rr)))

				action := GormActionSave
				if deleted {
					action = GormActionDelete
				}

				for _, item := range rr {
					pubGormAction(ctx, item, action)
				}

				processed += len(rr)
				index++
				if len(rr) < SyncPageSize {
					l.Info("no more data", zap.Int("processed", processed), zap.Bool("forDeleted", deleted))
					break
				}
			}
			return nil
		}
		l.Info("sync entities")
		err := fnitem(false)
		if err != nil {
			return err
		}

		l.Info("sync deleted entities")
		err = fnitem(true)

		return err
	})
}
