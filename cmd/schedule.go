/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/dig"
)

type Schedule struct {
	dig.In
	Startups []core.Startup `group:"startups"`
}

// scheduleCmd represents the schedule command
var ScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "start the schedule job only",
	RunE: func(cmd *cobra.Command, args []string) error {
		return core.GetContainer().Invoke(func(p Schedule) error {
			core.NotifyStarted()
			CloseOnlyNotified()
			return nil
		})
	},
}
