package sqlite

import (
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"gorm.io/driver/sqlite"
)

func init() {
	orm.DialectorMap["sqlite"] = sqlite.Open
	orm.DialectorMap["sqlite3"] = sqlite.Open
}
