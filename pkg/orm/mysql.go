// +build mysql
package orm

import "gorm.io/driver/mysql"

func init() {
	DialectorMap["mysql"] = mysql.Open
}
