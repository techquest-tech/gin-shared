package notify

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func TestEmailNotifier_ReloadsDBConfigOnDemand(t *testing.T) {
	db, err := gorm.Open(sqlite.Dialector{DSN: ":memory:", DriverName: "sqlite"}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&NotifyEmailConfig{}, &NotifyEmailTemplate{}))

	cfg := NotifyEmailConfig{
		Namespace: "bala",
		From:      "db@example.com",
		SmtpHost:  "smtp.db",
		SmtpPort:  25,
	}
	require.NoError(t, db.Create(&cfg).Error)
	tmpl := NotifyEmailTemplate{
		EmailConfigID: cfg.ID,
		Name:          "bala_orders_empty",
		Subject:       "v1",
		Body:          "body v1",
	}
	require.NoError(t, db.Create(&tmpl).Error)

	oldOpen := openDBForNotifyFn
	openDBForNotifyFn = func(_ string, _ string) (*gorm.DB, func(), error) {
		return db, func() {}, nil
	}
	t.Cleanup(func() {
		openDBForNotifyFn = oldOpen
		notifyDBMu.Lock()
		cachedNotifyDB = nil
		cachedNotifyDBDSN = ""
		cachedNotifyDBPref = ""
		notifyDBMu.Unlock()
	})

	viper.Set("database.connection", "sqlite")
	viper.Set("database.tablePrefix", "")

	en := &EmailNotifer{
		Logger: zap.NewNop(),
		From:   "yaml@example.com",
		SMTP: SmtpSettings{
			Host: "smtp.yaml",
			Port: 25,
		},
		Template: map[string]*EmailTmpl{
			"bala_orders_empty": {
				Subject: "yaml",
				Body:    "yaml",
			},
		},
	}

	require.NoError(t, en.PostInit())
	require.Equal(t, "v1", en.Template["bala_orders_empty"].Subject)

	require.NoError(t, db.Model(&NotifyEmailTemplate{}).Where("id = ?", tmpl.ID).Update("subject", "v2").Error)

	require.NoError(t, en.tryLoadFromDBOrSkip(context.Background()))
	require.NoError(t, en.initTemplatesAndAuth())
	require.Equal(t, "v2", en.Template["bala_orders_empty"].Subject)
}
