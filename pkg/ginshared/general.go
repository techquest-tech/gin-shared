package ginshared

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type GeneralResp struct {
	Succ         bool
	ErrorMessage string
	Data         interface{}
}

type ReportError struct {
	ErrorCode int
	logger    *zap.Logger
}

func (handle *ReportError) Middleware(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil {
			handle.logger.Error("error found", zap.Any("error", err))
			errorResp := GeneralResp{
				Succ:         false,
				ErrorMessage: fmt.Sprintf("%+v", err),
			}
			c.JSON(int(handle.ErrorCode), errorResp)
			c.Abort()
		}
	}()

	c.Next()
}

func NewErrorReport(errorCode int, logger *zap.Logger) gin.HandlerFunc {
	r := ReportError{
		ErrorCode: errorCode,
		logger:    logger,
	}
	return r.Middleware
}
