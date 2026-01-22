package orm

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type QueryBase struct {
	OwnerID   uint
	OwnerName string
	Page      int    `form:"page"`
	PageSize  int    `form:"pageSize"`
	Q         string `form:"q"`
	OrderBy   string `form:"orderBy"`
}

type PagingResult[T any] struct {
	Page      int
	PageSize  int
	TotalPage int64
	Total     int64
	Error     error
	Data      []T
}

func (p *PagingResult[T]) Trigger(tx *gorm.DB, req QueryBase) error {
	result := make([]T, 0)
	p.Page = req.Page
	p.PageSize = req.PageSize

	if req.PageSize > 0 {
		total := int64(0)
		err := tx.Count(&total).Error
		if err != nil {
			zap.L().Error("count total failed.", zap.Error(err))
			return err
		}
		p.Total = total
		if total > 0 {
			p.TotalPage = (total + int64(req.PageSize) - 1) / int64(req.PageSize)
		}
		tx = tx.Limit(req.PageSize).Offset(req.Page * req.PageSize)
	}

	err := tx.Debug().Find(&result).Error
	if err != nil {
		zap.L().Error("query task details failed.", zap.Error(err))
		return err
	}
	p.Data = result
	zap.L().Debug("query paging result done")
	if req.PageSize == 0 {
		p.Total = int64(len(p.Data))
	}
	return nil
}
