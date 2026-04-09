package orm

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type GetViewSql func(tablePrefix, view, query string) string

var ViewMap = make(map[string]GetViewSql)

func DefaulViewSql(tablePrefix, view, query string) string {
	return fmt.Sprintf(ViewMysqlTmp, tablePrefix, view, query)
}

var ReplaceTablePrefix = core.ReplaceTablePrefix

const ViewMysqlTmp = "CREATE OR REPLACE ALGORITHM = UNDEFINED VIEW %s%s AS %s"

func CleanViews(tx *gorm.DB, logger *zap.Logger, viewsToClean []string) error {
	if len(viewsToClean) == 0 {
		return nil
	}

	dbSettings := viper.Sub("database")
	if dbSettings == nil {
		return nil
	}

	tablePrefix := dbSettings.GetString("tablePrefix")
	viewSettings := dbSettings.Sub("views")
	if viewSettings == nil {
		return nil
	}

	cleanAll := false
	for _, v := range viewsToClean {
		if v == "*" {
			cleanAll = true
			break
		}
	}

	targetViews := make(map[string]bool)
	if !cleanAll {
		for _, v := range viewsToClean {
			targetViews[v] = true
		}
	}

	errs := make([]error, 0)
	for _, key := range viewSettings.AllKeys() {
		if !cleanAll && !targetViews[key] {
			continue
		}

		viewName := tablePrefix + key
		// Some databases might not support IF EXISTS in DROP VIEW, but most modern ones do (MySQL, Postgres, SQL Server etc.)
		dropSql := fmt.Sprintf("DROP VIEW IF EXISTS %s", viewName)
		
		err := tx.Exec(dropSql).Error
		if err != nil {
			logger.Error("drop view failed", zap.Error(err), zap.String("view", key), zap.String("viewName", viewName))
			errs = append(errs, err)
			continue
		}
		logger.Info("drop view done", zap.String("view", key), zap.String("viewName", viewName))
	}

	if len(errs) > 0 {
		return fmt.Errorf("clean views failed: %v", errs)
	}
	return nil
}

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
