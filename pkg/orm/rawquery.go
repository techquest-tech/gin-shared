package orm

import "gorm.io/gorm"

type RawQuery struct {
	Sql    string
	Preset map[string]interface{}
	Params []string
}

// var KeyPage = "page"
// var KeyPagesize = "pagesize"

// func (r *RawQuery) GetFromMap(data map[string]interface{}, key string) int {
// 	if obj, ok := data[key]; ok {
// 		if result, ok := obj.(int); ok {
// 			return result
// 		}
// 	}
// 	return 0
// }

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

	// page := r.GetFromMap(data, KeyPage)
	// pagesize := r.GetFromMap(data, KeyPagesize)

	// if pagesize > 0 {
	// 	tx = tx.Limit(pagesize).Offset(page * pagesize)
	// }

	err := tx.Find(&result).Error

	return result, err
}
