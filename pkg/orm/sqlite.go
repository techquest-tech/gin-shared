// +build sqlite
package orm

import "gorm.io/driver/sqlite"

func init() {
	DialectorMap["sqlite"] = sqlite.Open
	DialectorMap["sqlite3"] = sqlite.Open
}
