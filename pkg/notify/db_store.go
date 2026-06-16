package notify

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type EmailNotifierStore struct {
	db *gorm.DB
}

func NewEmailNotifierStore(db *gorm.DB) *EmailNotifierStore {
	return &EmailNotifierStore{db: db}
}

func (s *EmailNotifierStore) Upsert(ctx context.Context, namespace string, en *EmailNotifer) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("db is nil")
	}
	if namespace == "" {
		return fmt.Errorf("namespace is empty")
	}
	if en == nil {
		return fmt.Errorf("notifier is nil")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cfg NotifyEmailConfig
		// 不依赖数据库唯一索引约束，优先取最新一条配置记录，避免历史脏数据导致加载到旧配置。
		err := tx.Where("namespace = ?", namespace).Order("id desc").First(&cfg).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		cfg.Namespace = namespace
		cfg.From = en.From
		cfg.SmtpHost = en.SMTP.Host
		cfg.SmtpPort = en.SMTP.Port
		cfg.SmtpUsername = en.SMTP.Username
		cfg.SmtpPassword = en.SMTP.Password
		cfg.SmtpTLS = en.SMTP.Tls

		if err := tx.Save(&cfg).Error; err != nil {
			return err
		}

		if err := tx.Where("email_config_id = ?", cfg.ID).Delete(&NotifyEmailTemplate{}).Error; err != nil {
			return err
		}

		for name, t := range en.Template {
			if t == nil {
				continue
			}
			row := NotifyEmailTemplate{
				EmailConfigID: cfg.ID,
				Name:          name,
				Subject:       t.Subject,
				Body:          t.Body,
				Receivers:     StringSlice(t.Receivers),
				CcReceivers:   StringSlice(t.CcReceivers),
				BccReceivers:  StringSlice(t.BccReceivers),
				ReplyTo:       StringSlice(t.ReplyTo),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *EmailNotifierStore) Load(ctx context.Context, namespace string) (*EmailNotifer, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace is empty")
	}

	var cfg NotifyEmailConfig
	// 不依赖数据库唯一索引约束，优先取最新一条配置记录，避免历史脏数据导致加载到旧配置。
	if err := s.db.WithContext(ctx).Where("namespace = ?", namespace).Order("id desc").First(&cfg).Error; err != nil {
		return nil, err
	}

	var tmpls []NotifyEmailTemplate
	if err := s.db.WithContext(ctx).Where("email_config_id = ?", cfg.ID).Find(&tmpls).Error; err != nil {
		return nil, err
	}

	en := &EmailNotifer{
		From: cfg.From,
		SMTP: SmtpSettings{
			Host:     cfg.SmtpHost,
			Port:     cfg.SmtpPort,
			Username: cfg.SmtpUsername,
			Password: cfg.SmtpPassword,
			Tls:      cfg.SmtpTLS,
		},
		Template: map[string]*EmailTmpl{},
	}

	for _, row := range tmpls {
		en.Template[row.Name] = &EmailTmpl{
			Subject:       row.Subject,
			Body:          row.Body,
			Receivers:     []string(row.Receivers),
			CcReceivers:   []string(row.CcReceivers),
			BccReceivers:  []string(row.BccReceivers),
			ReplyTo:       []string(row.ReplyTo),
			tSub:          nil,
			tBody:         nil,
		}
	}

	return en, nil
}

func (en *EmailNotifer) LoadFromDB(ctx context.Context, db *gorm.DB, namespace string) error {
	store := NewEmailNotifierStore(db)
	loaded, err := store.Load(ctx, namespace)
	if err != nil {
		return err
	}

	en.From = loaded.From
	en.SMTP = loaded.SMTP
	en.Template = loaded.Template
	return en.PostInit()
}
