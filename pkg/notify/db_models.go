package notify

import (
	"time"

	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type NotifyEmailConfig struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Namespace string `gorm:"type:varchar(100);not null"`

	From         string `gorm:"type:varchar(255);not null"`
	SmtpHost     string `gorm:"type:varchar(255);not null"`
	SmtpPort     int    `gorm:"not null"`
	SmtpUsername string `gorm:"type:varchar(255)"`
	SmtpPassword string `gorm:"type:varchar(255)"`
	SmtpTLS      bool   `gorm:"not null;default:false"`
}

type NotifyEmailTemplate struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	EmailConfigID uint `gorm:"not null;index"`
	Name          string

	Subject string `gorm:"type:text"`
	Body    string `gorm:"type:text"`

	Receivers    StringSlice `gorm:"type:text"`
	CcReceivers  StringSlice `gorm:"type:text"`
	BccReceivers StringSlice `gorm:"type:text"`
	ReplyTo      StringSlice `gorm:"type:text"`
}

func init() {
	orm.AppendEntity(&NotifyEmailConfig{}, &NotifyEmailTemplate{})
	orm.RegisterPostMigrate(func(db *gorm.DB, logger *zap.Logger) error {
		if db == nil {
			return nil
		}

		ns := db.NamingStrategy
		cfgTable := ns.TableName("notify_email_configs")

		if err := orm.EnsureSoftDeleteUniqueIndexV(db, cfgTable, "namespace"); err != nil {
			if logger != nil {
				logger.Error("ensure unique index failed", zap.Error(err))
			}
			return err
		}
		return nil
	})
}
