package orm

import (
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DeleteDuplicatesKeepLastMultiFields 删除重复记录，保留最后一条, 可以指定多个字段
func DeleteDuplicatesKeepLastMultiFields[T any](db *gorm.DB, uniqueFields ...string) error {
	// 需要判断重复的字段列表（这里以 email, name, phone 为例）
	duplicateFields := uniqueFields

	// 1. 找出有重复的组合记录（返回所有重复组的完整记录）
	var duplicateRecords []map[string]any

	var v T

	err := db.Model(v).
		Select(duplicateFields).                    // 只选需要的字段用于分组
		Group(strings.Join(duplicateFields, ", ")). // 动态拼接分组字段
		Having("COUNT(*) > 1").
		Find(&duplicateRecords).Error
	if err != nil {
		return err
	}

	// 2. 遍历每一条重复组记录，删除该组中除最后一条外的记录
	for _, record := range duplicateRecords {
		var itemsToDelete []T

		err := db.Where(record).
			Order("id DESC"). // 或 "created_at DESC"
			Offset(1).        // 跳过最后一条（最新的）
			Find(&itemsToDelete).Error
		if err != nil {
			return err
		}

		for _, item := range itemsToDelete {
			zap.L().Info("delete duplicate item", zap.Any("item", item))
			db.Delete(&item) // 软删除，或 Unscoped() 硬删除
		}
	}

	return nil
}
