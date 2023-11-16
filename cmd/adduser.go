/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/auth"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/thanhpk/randstr"
)

var (
	username string
	owner    string
	remark   string
	apikey   string
)

// adduserCmd represents the adduser command
var AdduserCmd = &cobra.Command{
	Use:   "adduser",
	Short: "add API key for owner",
	RunE: func(cmd *cobra.Command, args []string) error {
		return core.GetContainer().Invoke(func(auth *auth.AuthService) error {
			if username == "" {
				username = owner + "-user" + randstr.Dec(3)
			}
			key, err := auth.CreateUser(owner, username, remark, apikey)
			if err != nil {
				return err
			}
			fmt.Println("API key created.", key)
			return nil
		})
	},
}

func init() {
	// rootCmd.AddCommand(adduserCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// adduserCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// adduserCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	AdduserCmd.Flags().StringVarP(&username, "username", "u", "", "API Key name")
	AdduserCmd.Flags().StringVarP(&owner, "owner", "o", "", "Ownername")
	AdduserCmd.Flags().StringVarP(&remark, "remark", "", "", "remark only")
	AdduserCmd.Flags().StringVarP(&apikey, "raw", "k", "", "leave it empty will random one.")
}
