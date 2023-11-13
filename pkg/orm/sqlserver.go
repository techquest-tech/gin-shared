//go:build sqlserver || all

package orm

import (
	"fmt"

	"gorm.io/driver/sqlserver"
)

func init() {
	DialectorMap["sqlserver"] = sqlserver.Open
	ViewMap["sqlserver"] = func(tablePrefix, view, query string) string {
		return fmt.Sprintf("CREATE OR ALTER VIEW %s%s as %s", tablePrefix, view, query)
	}
}
