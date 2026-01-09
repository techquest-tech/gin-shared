/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/asaskevich/EventBus"
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// initDBCmd represents the initDB command
var InitDBCmd = &cobra.Command{
	Use:   "initDB",
	Short: "init tables",
	RunE: func(cmd *cobra.Command, args []string) error {
		return core.Container.Invoke(func(db *gorm.DB, logger *zap.Logger, bus EventBus.Bus) {
			orm.MigrateTableAndView(db, logger, bus)
		})
	},
}

func init() {
	// rootCmd.AddCommand(initDBCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initDBCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initDBCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
