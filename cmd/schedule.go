/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/schedule"
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
			core.NotifyStopping()
			return nil
		})
	},
}

var RunJobCmd = &cobra.Command{
	Use:   "job",
	Short: "run job now",
	RunE: func(cmd *cobra.Command, args []string) error {
		return core.GetContainer().Invoke(func(p Schedule) error {
			if len(args) == 0 {
				keys := schedule.List()
				println("available jobs:")
				for _, k := range keys {
					println(k)
				}
				return nil
			}
			return schedule.Run(args[0])
		})
	},
}
