package orm

import (
	"bytes"
	"fmt"
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
				return err
			}

			raw := viewSql(tablePrefix, key, out.String())
			err = tx.Exec(raw).Error
			if err != nil {
				logger.Error("update view failed", zap.Error(err), zap.String("view", key))
				return err
			}
			logger.Info("create view done", zap.String("view", key))
		}
	}
	return nil
}
