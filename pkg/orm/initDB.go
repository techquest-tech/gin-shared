package orm

import (
	"fmt"
	"runtime/debug"

	"github.com/asaskevich/EventBus"
	"github.com/spf13/cobra"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// runInitDBWithReport 执行 initDB 主体，并把异常上报到 error 通道：
//   - orm.InitDB 连不上库 / 驱动缺失时是直接 panic（见 database.go），initDB
//     命令没有 gin recover 兜底，这里捕获、上报（带 panic 堆栈）后转成普通
//     error 返回，进程以非 0 退出而不是甩一屏堆栈；
//   - 容器 Invoke 失败（如依赖缺失）等普通 error 同样上报后再原样返回。
func runInitDBWithReport(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("initDB panic: %v", r)
			reportInitException(err, InitDBUri+"/panic", debug.Stack())
		}
	}()

	if err = fn(); err != nil {
		reportInitException(err, InitDBUri, nil)
	}
	return err
}

// initDBCmd represents the initDB command
var InitDBCmd = &cobra.Command{
	Use:   "initDB",
	Short: "init tables",
	RunE: func(cmd *cobra.Command, args []string) error {
		cleanViews, _ := cmd.Flags().GetStringSlice("cleanViews")

		return runInitDBWithReport(func() error {
			return core.Container.Invoke(func(db *gorm.DB, logger *zap.Logger, bus EventBus.Bus) {
				MigrateTableAndView(db, logger, bus, cleanViews...)
			})
		})
	},
}

func init() {
	// rootCmd.AddCommand(initDBCmd)
	InitDBCmd.Flags().StringSliceP("cleanViews", "c", []string{}, "Clean specified views before migration. Use '*' to clean all pending views. For postgres, default is '*'.")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initDBCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initDBCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
