package query

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RawQueryWhere struct {
	Cond  string
	Param string
	Empty string // optional settings, where condition for params is empty
}

type RawQuery struct {
	Sql     string
	Preset  map[string]any
	Params  []string
	Where   map[string]string // key: where condition, value: param key, e.g. "id = ?", "id"
	Orderby string
	Groupby string
}

func (r *RawQuery) Query(db *gorm.DB, data map[string]any) ([]map[string]any, error) {
	allParams := map[string]any{}

	sql := r.Sql

	for k, v := range r.Preset {
		allParams[k] = v
	}
	for k, v := range data {
		allParams[k] = v
	}
	params := make([]any, 0)
	for _, key := range r.Params {
		params = append(params, allParams[key])
	}

	if len(r.Where) > 0 {
		hasWhere := strings.Contains(r.Sql, "where")
		if !hasWhere {
			sql = sql + " where "
		}
		conds := make([]string, 0)
		for cond, p := range r.Where {
			if v, ok := allParams[p]; ok {
				if s, ok := v.(string); ok {
					ss := strings.TrimSpace(s)
					if ss == "" {
						zap.L().Debug("empty value ", zap.String("param", p))
						continue
					}
				}
				conds = append(conds, cond)
				params = append(params, v)
			}
		}
		if len(conds) > 0 {
			sql = sql + strings.Join(conds, " and ")
		}
	}
	if r.Groupby != "" {
		sql = sql + " group by " + r.Groupby
	}

	if r.Orderby != "" {
		sql = fmt.Sprintf("%s order by %s", sql, r.Orderby)
	}

	result := make([]map[string]any, 0)

	tx := db.Raw(sql, params...)

	err := tx.Find(&result).Error

	return result, err
}
