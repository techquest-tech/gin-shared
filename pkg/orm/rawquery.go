package orm

import "gorm.io/gorm"

type RawQuery struct {
	Sql    string
	Preset map[string]interface{}
	Params []string
}

func (r *RawQuery) Query(db *gorm.DB, data map[string]interface{}) ([]map[string]interface{}, error) {
	allParams := map[string]interface{}{}

	for k, v := range r.Preset {
		allParams[k] = v
	}
	for k, v := range data {
		allParams[k] = v
	}
	params := make([]interface{}, 0)
	for _, key := range r.Params {
		params = append(params, allParams[key])
	}
	result := make([]map[string]interface{}, 0)

	tx := db.Raw(r.Sql, params...)

	err := tx.Find(&result).Error

	return result, err
}
