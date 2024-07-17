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

type GormObjSyncService struct {
	MessageService MessagingService
	DB             *gorm.DB
	Logger         *zap.Logger
}

var cfg = &gorm.Session{
	SkipHooks: true,
}

func (ss *GormObjSyncService) ReceiveGormObjectSaved(ctx context.Context, topic, consumer string, raw []byte) error {
	key, payloadRaw, err := ToKeyAndPayload(raw)
	if err != nil {
		ss.Logger.Error("unexpected payload", zap.Error(err))
		return err
	}
	tt, ok := m[key]
	if !ok {
		return errors.New("received object failed. unknown key " + key)
	}
	payload := reflect.New(tt).Interface()
	err = json.Unmarshal(payloadRaw, payload)
	if err != nil {
		ss.Logger.Info("unexpected payload,", zap.Error(err))
		return err
	}
	err = ss.DB.Session(cfg).Save(payload).Error
	if err != nil {
		ss.Logger.Info("save object failed.", zap.Error(err), zap.String("data", tt.Name()), zap.Any("payload", payload))
		return err
	}
	ss.Logger.Info("save object done.", zap.String("data", tt.Name()))
	return nil
}

type PayloadForGormSaved struct {
	Key     string
	Payload []byte
}

func ToKeyAndPayload(raw []byte) (string, []byte, error) {
	var payload PayloadForGormSaved
	err := json.Unmarshal(raw, &payload)
	if err != nil {
		return "", nil, err
	}
	return payload.Key, payload.Payload, nil
}

var m = map[string]reflect.Type{}
var revert = map[reflect.Type]string{}

func Reg(payload any) {
	tt := reflect.TypeOf(payload)
	// if key == "" {
	key := tt.String()
	key = strings.TrimLeft(key, "*")
	// }
	m[key] = tt
	revert[tt] = key

	zap.L().Info("gorm object registered.", zap.String("key", key), zap.String("type", tt.String()))
}

func PubGormSaved(ctx context.Context, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// if key == "" {
	tt := reflect.TypeOf(payload)
	key := tt.String()
	key = strings.TrimLeft(key, "*")
	// }

	return ms.Pub(ctx, DefaultGormToipc, PayloadForGormSaved{Key: key, Payload: raw})
}
