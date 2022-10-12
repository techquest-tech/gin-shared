//go:build sqlite || all

package orm

import (
	"fmt"
	"gorm.io/driver/sqlite"
)

func init() {
	DialectorMap["sqlite"] = sqlite.Open
	ViewMap["sqlite"] = func(tablePrefix, view, query string) string {
		return fmt.Sprintf("CREATE VIEW IF NOT EXISTS %s%s as %s", tablePrefix, view, query)
	}
}
