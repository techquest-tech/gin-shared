package orm

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type GetViewSql func(tablePrefix, view, query string) string

var ViewMap = make(map[string]GetViewSql)

func DefaulViewSql(tablePrefix, view, query string) string {
	return fmt.Sprintf(ViewMysqlTmp, tablePrefix, view, query)
}

func ReplaceTablePrefix(raw string, prefixes ...string) string {
	prefix := viper.GetString("database.tablePrefix")
	if len(prefixes) > 0 {
		prefix = prefixes[0]
	}
	return strings.ReplaceAll(raw, "{{.tableprefix}}", prefix)
}

const ViewMysqlTmp = "CREATE OR REPLACE ALGORITHM = UNDEFINED VIEW %s%s AS %s"

func InitMysqlViews(tx *gorm.DB, logger *zap.Logger) error {

	dbSettings := viper.Sub("database")

	tablePrefix := dbSettings.GetString("tablePrefix")

	dbtype := dbSettings.GetString("type")

	viewSql, ok := ViewMap[dbtype]
	if !ok {
		viewSql = DefaulViewSql
	}

	viewSettings := dbSettings.Sub("views")
	errs := make([]error, 0)
	if viewSettings != nil {
		for _, key := range viewSettings.AllKeys() {
			query := viewSettings.GetString(key)

			data := make(map[string]interface{}, 0)
			data["tableprefix"] = tablePrefix
			viewTpl := template.Must(template.New("tableprefix").Parse(query))
			out := bytes.Buffer{}
			err := viewTpl.Execute(&out, data)
			if err != nil {
				logger.Error("match view template failed.", zap.Error(err))
				// return err
				errs = append(errs, err)
				continue
			}

			raw := viewSql(tablePrefix, key, out.String())
			err = tx.Exec(raw).Error
			if err != nil {
				logger.Error("update view failed", zap.Error(err), zap.String("view", key))
				// return err
				errs = append(errs, err)
				continue
			}
			logger.Info("create view done", zap.String("view", key))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("update view failed: %v", errs)
	}
	return nil
}
