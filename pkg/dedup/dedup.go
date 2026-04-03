package dedup

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HashBytesProvider interface {
	HashBytes() []byte
}

type ObjectFingerprint struct {
	gorm.Model
	ObjectKey  string `gorm:"size:255;not null;uniqueIndex:uk_object_dedup_key_name,priority:1"`
	ObjectName string `gorm:"size:255;not null;uniqueIndex:uk_object_dedup_key_name,priority:2"`
	MD5        string `gorm:"size:32;not null;index"`
}

type ObjectFingerprintService struct {
	db *gorm.DB
}

func NewObjectFingerprintService(db *gorm.DB) *ObjectFingerprintService {
	return &ObjectFingerprintService{db: db}
}

func (s *ObjectFingerprintService) Set(objectKey string, objectName string, obj any) (*ObjectFingerprint, error) {
	md5Value, err := BuildObjectMD5(obj)
	if err != nil {
		return nil, fmt.Errorf("build md5 failed key=%s name=%s: %w", objectKey, objectName, err)
	}
	item := &ObjectFingerprint{
		ObjectKey:  objectKey,
		ObjectName: objectName,
		MD5:        md5Value,
	}
	err = s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "object_key"},
			{Name: "object_name"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"md5":        md5Value,
			"updated_at": gorm.Expr("now()"),
			"deleted_at": nil,
		}),
	}).Create(item).Error
	if err != nil {
		return nil, fmt.Errorf("save dedup failed key=%s name=%s: %w", objectKey, objectName, err)
	}
	return item, nil
}

func (s *ObjectFingerprintService) Get(objectKey string, objectName string) (*ObjectFingerprint, error) {
	item := &ObjectFingerprint{}
	err := s.db.Where("object_key = ? and object_name = ?", objectKey, objectName).First(item).Error
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ObjectFingerprintService) IsDuplicated(objectKey string, objectName string, obj any) (bool, error) {
	current, err := BuildObjectMD5(obj)
	if err != nil {
		return false, fmt.Errorf("build md5 failed key=%s name=%s: %w", objectKey, objectName, err)
	}
	item, err := s.Get(objectKey, objectName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("load dedup record failed key=%s name=%s: %w", objectKey, objectName, err)
	}
	return item.MD5 == current, nil
}

func BuildObjectMD5(obj any) (string, error) {
	if obj == nil {
		return "", fmt.Errorf("obj is nil")
	}
	var payload []byte
	if v, ok := obj.(HashBytesProvider); ok {
		payload = v.HashBytes()
	} else {
		raw, err := json.Marshal(obj)
		if err != nil {
			return "", err
		}
		payload = raw
	}
	sum := md5.Sum(payload)
	return hex.EncodeToString(sum[:]), nil
}

var ServiceObjectFingerprint *ObjectFingerprintService

func init() {
	core.Provide(NewObjectFingerprintService)
	core.ProvideStartup(func(s *ObjectFingerprintService) core.Startup {
		ServiceObjectFingerprint = s
		return nil
	})
}
