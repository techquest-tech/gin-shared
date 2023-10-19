/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

// genconfigCmd represents the genconfig command
var GenconfigCmd = &cobra.Command{
	Use:   "genconfig",
	Short: "generate all embed config to " + core.EmbedConfigFile,

	Run: func(cmd *cobra.Command, args []string) {
		core.GetContainer().Invoke(func(logger *zap.Logger) error {
			err := core.GenerateEmbedConfigfile()
			if err != nil {
				logger.Error("write file failed.", zap.Error(err))
				return err
			}
			logger.Info("Done")
			return nil
		})
	},
}
