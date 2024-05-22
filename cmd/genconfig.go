/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

// genconfigCmd represents the genconfig command
var GenconfigCmd = &cobra.Command{
	Use:   "genconfig",
	Short: "no need to gen config any more. ", // + core.EmbedConfigFile,

	Run: func(cmd *cobra.Command, args []string) {
		// core.GetContainer().Invoke(func(logger *zap.Logger) error {
		// 	err := core.GenerateEmbedConfigfile()
		// 	if err != nil {
		// 		logger.Error("write file failed.", zap.Error(err))
		// 		return err
		// 	}
		// 	logger.Info("Done")
		// 	return nil
		// })
	},
}

var EncryptConfig = &cobra.Command{
	Use:   "encrypt",
	Short: "encrypt all configs to a single file ",

	Run: func(cmd *cobra.Command, args []string) {
		os.Remove(core.EncryptedFile)
		fmt.Printf("delete %s if existed\n", core.EncryptedFile)
		core.GetContainer().Invoke(func(logger *zap.Logger) error {
			return core.EncryptConfig()
		})
	},
}
