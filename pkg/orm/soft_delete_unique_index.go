package orm

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

func EnsureSoftDeleteUniqueIndex(db *gorm.DB, table string, uniqueColumns []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	table = strings.TrimSpace(table)
	if table == "" {
		return fmt.Errorf("table is empty")
	}
	if len(uniqueColumns) == 0 {
		return fmt.Errorf("unique columns is empty")
	}

	dialect := ""
	if db.Dialector != nil {
		dialect = strings.ToLower(strings.TrimSpace(db.Dialector.Name()))
	}

	cols := make([]string, 0, len(uniqueColumns))
	for _, c := range uniqueColumns {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		cols = append(cols, c)
	}
	if len(cols) == 0 {
		return fmt.Errorf("unique columns is empty")
	}

	indexName := buildSoftDeleteUniqueIndexName(table, cols)

	switch dialect {
	case "postgres":
		ddl := fmt.Sprintf(
			`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE %s IS NULL`,
			quoteIdent(dialect, indexName),
			quoteTable(dialect, table),
			joinQuotedIdents(dialect, cols),
			quoteIdent(dialect, "deleted_at"),
		)
		return db.Exec(ddl).Error
	case "sqlite":
		ddl := fmt.Sprintf(
			`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE %s IS NULL`,
			quoteIdent(dialect, indexName),
			quoteTable(dialect, table),
			joinQuotedIdents(dialect, cols),
			quoteIdent(dialect, "deleted_at"),
		)
		return db.Exec(ddl).Error
	case "mysql":
		exists, err := mysqlIndexExists(db, table, indexName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		ddl := fmt.Sprintf(
			`CREATE UNIQUE INDEX %s ON %s (%s, ((IF(%s IS NULL, 0, NULL))))`,
			quoteIdent(dialect, indexName),
			quoteTable(dialect, table),
			joinQuotedIdents(dialect, cols),
			quoteIdent(dialect, "deleted_at"),
		)
		return db.Exec(ddl).Error
	case "sqlserver":
		exists, err := sqlserverIndexExists(db, table, indexName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		ddl := fmt.Sprintf(
			`CREATE UNIQUE INDEX %s ON %s (%s) WHERE %s IS NULL`,
			quoteIdent(dialect, indexName),
			quoteTable(dialect, table),
			joinQuotedIdents(dialect, cols),
			quoteIdent(dialect, "deleted_at"),
		)
		return db.Exec(ddl).Error
	default:
		return nil
	}
}

func EnsureSoftDeleteUniqueIndexV(db *gorm.DB, table string, uniqueColumns ...string) error {
	return EnsureSoftDeleteUniqueIndex(db, table, uniqueColumns)
}

func joinQuotedIdents(dialect string, cols []string) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		parts = append(parts, quoteIdent(dialect, c))
	}
	return strings.Join(parts, ", ")
}

func quoteTable(dialect, table string) string {
	parts := strings.Split(table, ".")
	for i := range parts {
		parts[i] = quoteIdent(dialect, parts[i])
	}
	return strings.Join(parts, ".")
}

func quoteIdent(dialect, ident string) string {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return ident
	}
	switch dialect {
	case "mysql":
		if strings.HasPrefix(ident, "`") && strings.HasSuffix(ident, "`") {
			return ident
		}
		return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
	case "sqlserver":
		if strings.HasPrefix(ident, "[") && strings.HasSuffix(ident, "]") {
			return ident
		}
		return "[" + strings.ReplaceAll(ident, "]", "]]") + "]"
	default:
		if strings.HasPrefix(ident, `"`) && strings.HasSuffix(ident, `"`) {
			return ident
		}
		return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
	}
}

func buildSoftDeleteUniqueIndexName(table string, cols []string) string {
	baseTable := unqualifiedTableName(table)
	raw := "uidx_" + baseTable + "_" + strings.Join(cols, "_") + "_active"
	name := sanitizeIndexName(raw)
	if len(name) <= 60 {
		return name
	}
	h := sha1.Sum([]byte(name))
	return sanitizeIndexName(name[:40] + "_" + hex.EncodeToString(h[:])[:16])
}

func unqualifiedTableName(table string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		return table
	}
	parts := strings.Split(table, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

var indexNameRe = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func sanitizeIndexName(name string) string {
	name = strings.TrimSpace(name)
	name = indexNameRe.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "uidx_active"
	}
	return strings.ToLower(name)
}

func mysqlIndexExists(db *gorm.DB, table, indexName string) (bool, error) {
	tbl := unqualifiedTableName(table)
	var cnt int64
	if err := db.Raw(
		`SELECT COUNT(1) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`,
		tbl,
		indexName,
	).Scan(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func sqlserverIndexExists(db *gorm.DB, table, indexName string) (bool, error) {
	var cnt int64
	if err := db.Raw(
		`SELECT COUNT(1) FROM sys.indexes WHERE name = ? AND object_id = OBJECT_ID(?)`,
		indexName,
		table,
	).Scan(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}
