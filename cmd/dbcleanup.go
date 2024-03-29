/*
Copyright © 2022 Armen Pan <panarm@esquel.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/schedule"
)

var tables = make([]string, 0)
var col = ""
var duration = ""

// versionCmd represents the version command
var CleanupCmd = &cobra.Command{
	Use:   "dbcleanup",
	Short: "run DB clean up",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ginshared.GetContainer().Invoke(schedule.DoCleanup("cleanup", tables, col, duration))
	},
}

func init() {
	CleanupCmd.PersistentFlags().StringArrayVarP(&tables, "table", "t", []string{}, "tables to be cleanup, or tables in yaml file")
	CleanupCmd.PersistentFlags().StringVarP(&col, "col", "c", "", "col name for the duration")
	CleanupCmd.PersistentFlags().StringVarP(&duration, "duration", "d", "", "data older than duration")
}
