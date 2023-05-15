package sqlserver

import (
	"fmt"

	"github.com/techquest-tech/gin-shared/pkg/orm"

	_ "github.com/techquest-tech/gin-shared/pkg/query"
	"gorm.io/driver/sqlserver"
)

func init() {
	orm.DialectorMap["sqlserver"] = sqlserver.Open
	orm.ViewMap["sqlserver"] = func(tablePrefix, view, query string) string {
		return fmt.Sprintf("CREATE OR ALTER VIEW %s%s as %s", tablePrefix, view, query)
	}
}
