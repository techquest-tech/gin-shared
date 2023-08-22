/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/auth"
)

// hashCmd represents the hash command
var HashCmd = &cobra.Command{
	Use:   "hash",
	Short: "hash raw password for api key",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		rawKey := ""
		if len(args) == 1 {
			rawKey = args[0]
		} else {
			fmt.Print("enter raw api key(len(rawKey) > 5):")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println("Error reading input:", err)
				return
			}
			if len(input) <= 1 {
				return
			}
			rawKey = input[:len(input)-1]
		}

		if len(rawKey) < 5 {
			return
		}

		fmt.Println("going to hash ", rawKey[:2]+"****"+rawKey[len(rawKey)-2:])

		hashed := auth.Hash(rawKey)
		fmt.Print(hashed)
	},
}
