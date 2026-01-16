package orm

import (
	"strings"

	_ "github.com/alexbrainman/odbc"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

func regWithOdbc(typeName string) {
	dbtype := strings.TrimSuffix(typeName, ".odbc")
	DialectorMap[typeName] = func(dsn string) gorm.Dialector {
		// db, err := sql.Open("odbc", dsn)
		// if err != nil {
		// 	panic(err)
		// }
		switch dbtype {
		case "sqlserver":
			return sqlserver.Dialector{
				Config: &sqlserver.Config{
					DSN:        dsn,
					DriverName: "odbc",
				},
			}
		case "mysql":
			return mysql.Dialector{
				Config: &mysql.Config{
					DSN:        dsn,
					DriverName: "odbc",
				},
			}
		default:
			panic("not support dbtype: " + dbtype)
		}
	}
}

func init() {
	regWithOdbc("mysql.odbc")
	regWithOdbc("sqlserver.odbc")
}
