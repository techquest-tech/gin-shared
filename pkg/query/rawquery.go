package query

import (
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const KeyWhere = "{{.where}}"

// it's not enabled yet.
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
	Limit   int
	Offset  int
}

func (r *RawQuery) Query(db *gorm.DB, data map[string]any) ([]map[string]any, error) {
	return Query[map[string]any](db, r, data)
}

func Query[T any](db *gorm.DB, r *RawQuery, data map[string]any) ([]T, error) {
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
		conds := make([]string, 0)
		for cond, p := range r.Where {
			if v, ok := allParams[p]; ok {
				if s, ok := v.(string); ok {
					ss := strings.TrimSpace(s)
					if ss == "" || ss == "null" || ss == "-" || ss == "none" {
						zap.L().Debug("empty value ", zap.String("param", p))
						continue
					}
				}
				conds = append(conds, cond)
				params = append(params, v)
			}
		}
		if len(conds) > 0 {
			conds := strings.Join(conds, " and ")
			if strings.Contains(sql, KeyWhere) {
				sql = strings.Replace(sql, KeyWhere, conds, 1)
			} else {
				sql = sql + " where " + conds
			}
		}
	}

	groupby := r.Groupby
	if g, ok := data["groupby"]; ok {
		groupby = g.(string)
	}

	if groupby != "" {
		sql = sql + " group by " + groupby
	}

	orderby := r.Orderby
	if o, ok := data["orderby"]; ok {
		orderby = o.(string)
	}
	if orderby != "" {
		sql = fmt.Sprintf("%s order by %s", sql, orderby)
	}

	limit := r.Limit
	if l, ok := data["limit"]; ok {
		ll, err := strconv.Atoi(l.(string))
		if err != nil {
			zap.L().Warn("invalid limit", zap.Error(err), zap.String("limit", l.(string)))
		} else {
			limit = ll
		}
	}

	if limit > 0 {
		sql = fmt.Sprintf("%s limit %d", sql, limit)
	}

	offset := r.Offset
	if o, ok := data["offset"]; ok {
		oo, err := strconv.Atoi(o.(string))
		if err != nil {
			zap.L().Warn("invalid offset", zap.Error(err), zap.String("offset", o.(string)))
		} else {
			offset = oo
		}
	}
	if offset > 0 {
		sql = fmt.Sprintf("%s offset %d", sql, offset)
	}

	result := make([]T, 0)

	tx := db.Raw(sql, params...)

	err := tx.Find(&result).Error

	return result, err
}
