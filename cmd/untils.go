package cmd

import (
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
)

// var once sync.Once

var CloseOnlyNotified = core.CloseOnlyNotified

func ApplyEnvParams(c *cobra.Command) {
	c.PersistentFlags().StringSliceVarP(&core.EnvValues, "env", "e", []string{}, "set env values")
}
