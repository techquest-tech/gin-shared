package orm

import (
	"database/sql"
	"strings"

	_ "github.com/alexbrainman/odbc"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

func regWithOdbc(typeName string) {
	dbtype := strings.TrimSuffix(typeName, ".odbc")
	DialectorMap[dbtype] = func(dsn string) gorm.Dialector {
		db, err := sql.Open("odbc", dsn)
		if err != nil {
			panic(err)
		}
		switch dbtype {
		case "sqlserver":
			return sqlserver.Dialector{
				Config: &sqlserver.Config{
					DSN:  dsn,
					Conn: db,
				},
			}
		default:
			panic("not support dbtype")
		}
	}
}

func init() {
	regWithOdbc("sqlserver.odbc")
}
