//go:build pg || all
package orm

import (
	"fmt"

	"gorm.io/driver/postgres"
)

func init() {
	DialectorMap["postgres"] = postgres.Open
	ViewMap["postgres"] = func(tablePrefix, view, query string) string {
		return fmt.Sprintf("CREATE OR REPLACE VIEW %s%s as %s", tablePrefix, view, query)
	}
}
