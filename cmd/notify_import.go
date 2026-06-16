package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/notify"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var NotifyImportCmd = &cobra.Command{
	Use:   "notifyImport",
	Short: "import email notifier settings from yaml into database",
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		namespace, _ := cmd.Flags().GetString("namespace")
		configKey, _ := cmd.Flags().GetString("configKey")

		if file == "" {
			return fmt.Errorf("file is required")
		}
		if namespace == "" {
			return fmt.Errorf("namespace is required")
		}
		if configKey == "" {
			configKey = namespace + ".notifier"
		}

		v := viper.New()
		v.SetConfigFile(file)
		if err := v.ReadInConfig(); err != nil {
			return err
		}

		var en notify.EmailNotifer
		if err := v.UnmarshalKey(configKey, &en); err != nil {
			return err
		}

		return core.Container.Invoke(func(db *gorm.DB, logger *zap.Logger) error {
			if logger != nil {
				logger.Info("import notify settings", zap.String("namespace", namespace), zap.String("configKey", configKey), zap.Int("templates", len(en.Template)))
			}
			store := notify.NewEmailNotifierStore(db)
			return store.Upsert(context.Background(), namespace, &en)
		})
	},
}

func init() {
	NotifyImportCmd.Flags().StringP("file", "f", "", "yaml config file path")
	NotifyImportCmd.Flags().StringP("namespace", "n", "", "namespace/module name for db")
	NotifyImportCmd.Flags().String("configKey", "", "viper key path to notifier config, default is <namespace>.notifier")
}

