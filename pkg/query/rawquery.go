package query

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	KeyWhere = "{{.where}}"
)

type PagingResult[T any] struct {
	Page      int
	PageSize  int
	TotalPage int64
	Total     int64
	Error     error `json:",omitempty"`
	Data      []T   `json:",omitempty"`
}

var (
	PageSize = 100 // default page size
)

// it's not enabled yet.
type RawQueryWhere struct {
	Cond  string
	Param string
	Empty string // optional settings, where condition for params is empty
}

type RawQuery struct {
	Sql        string
	SumRef     string // reference to another RawQuery define, not used yet.
	SumEnabled bool
	Preset     map[string]any
	Params     []string
	Where      map[string]string // key: where condition, value: param key, e.g. "id = ?", "id"
	Orderby    string
	Groupby    string
}

func (r *RawQuery) Query(db *gorm.DB, data map[string]any) ([]map[string]any, error) {
	return Query[map[string]any](db, r, data)
}

// check should return Paging Result
func (r *RawQuery) ShouldPagingResult() bool {
	return r.SumRef != "" || r.SumEnabled
}

func (r *RawQuery) PagingResult(db *gorm.DB, result []map[string]any, req map[string]any) (*PagingResult[map[string]any], error) {
	pageSize := toInt(req, "page_size")
	page := toInt(req, "page")
	if pageSize == 0 {
		pageSize = PageSize
	}

	resp := &PagingResult[map[string]any]{
		Page:     page,
		PageSize: pageSize,
		Data:     result,
		Total:    int64(len(result)),
	}

	if len(result) == 0 {
		resp.TotalPage = 0
		return resp, nil
	}

	if len(result) < pageSize {
		resp.TotalPage = 1
		return resp, nil
	}
	total, err := r.sum(db, req)
	if err != nil {
		return nil, err
	}
	pageTotal := (total + int64(pageSize-1)) / int64(pageSize)

	resp.TotalPage = pageTotal
	return resp, nil
}

func (r *RawQuery) where(allParams map[string]any, params []any, sql string) (string, []any, error) {
	conds := make([]string, 0, len(r.Where))
	for p, cond := range r.Where {
		if v, ok := allParams[p]; ok {
			if s, ok := v.(string); ok {
				ss := strings.TrimSpace(s)
				if ss == "" || ss == "null" || ss == "-" || ss == "none" {
					zap.L().Debug("empty value ", zap.String("param", p))
					continue
				}
				v = ss
			}
			conds = append(conds, cond)
			if strings.Contains(cond, "like") {
				s, ok := v.(string)
				if !ok {
					return sql, params, errors.New("like param must be string")
				}
				if !strings.HasPrefix(s, "%") {
					s = s + "%"
				}
				params = append(params, s)
			} else {
				params = append(params, v)
			}

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
	return sql, params, nil
}

func (r *RawQuery) sum(db *gorm.DB, data map[string]any) (int64, error) {
	allParams := map[string]any{}
	maps.Copy(allParams, r.Preset)
	maps.Copy(allParams, data)

	params := make([]any, 0)
	for _, key := range r.Params {
		params = append(params, allParams[key])
	}
	// sql := r.SumSql
	// if sql == "" {
	// start:=strings.Index(r.Sql, "select ")
	end := strings.Index(r.Sql, "from")
	sql := "select count(1) as sum " + r.Sql[end:]
	if len(r.Where) > 0 {
		var err error
		sql, params, err = r.where(allParams, params, sql)
		if err != nil {
			return 0, err
		}
		// conds := make([]string, 0)
		// for p, cond := range r.Where {
		// 	if v, ok := allParams[p]; ok {
		// 		if s, ok := v.(string); ok {
		// 			ss := strings.TrimSpace(s)
		// 			if ss == "" || ss == "null" || ss == "-" || ss == "none" {
		// 				zap.L().Debug("empty value ", zap.String("param", p))
		// 				continue
		// 			}
		// 		}
		// 		conds = append(conds, cond)
		// 		params = append(params, v)
		// 	}
		// }
		// if len(conds) > 0 {
		// 	conds := strings.Join(conds, " and ")
		// 	if strings.Contains(sql, KeyWhere) {
		// 		sql = strings.Replace(sql, KeyWhere, conds, 1)
		// 	} else {
		// 		sql = sql + " where " + conds
		// 	}
		// }
	}
	// }

	count := int64(0)
	tx := db.Raw(sql, params...)

	err := tx.Find(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func Query[T any](db *gorm.DB, r *RawQuery, data map[string]any) ([]T, error) {
	allParams := map[string]any{}

	sql := r.Sql

	maps.Copy(allParams, r.Preset)
	maps.Copy(allParams, data)

	params := make([]any, 0)
	for _, key := range r.Params {
		params = append(params, allParams[key])
	}

	if len(r.Where) > 0 {
		var err error
		sql, params, err = r.where(allParams, params, sql)
		if err != nil {
			return nil, err
		}
		// conds := make([]string, 0)
		// for p, cond := range r.Where {
		// 	if v, ok := allParams[p]; ok {
		// 		if s, ok := v.(string); ok {
		// 			ss := strings.TrimSpace(s)
		// 			if ss == "" || ss == "null" || ss == "-" || ss == "none" {
		// 				zap.L().Debug("empty value ", zap.String("param", p))
		// 				continue
		// 			}
		// 		}
		// 		conds = append(conds, cond)
		// 		params = append(params, v)
		// 	}
		// }
		// if len(conds) > 0 {
		// 	conds := strings.Join(conds, " and ")
		// 	if strings.Contains(sql, KeyWhere) {
		// 		sql = strings.Replace(sql, KeyWhere, conds, 1)
		// 	} else {
		// 		sql = sql + " where " + conds
		// 	}
		// }
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

	limit := toInt(data, "limit")

	page := toInt(data, "page")
	pageSize := toInt(data, "page_size")

	if limit == 0 && pageSize > 0 {
		limit = pageSize
	}

	if limit > 0 {
		sql = fmt.Sprintf("%s limit %d", sql, limit)
	}

	offset := toInt(data, "offset")
	if offset == 0 && page > 0 && pageSize > 0 {
		offset = page * pageSize
	}
	if offset > 0 {
		sql = fmt.Sprintf("%s offset %d", sql, offset)
	}

	result := make([]T, 0)

	tx := db.Raw(sql, params...)

	err := tx.Find(&result).Error

	return result, err
}

func toInt(data map[string]any, key string) int {
	if v, ok := data[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
		if i, err := strconv.Atoi(v.(string)); err == nil {
			return i
		}
	}
	return 0
}
