package ginshared

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UnifiedResp struct {
	Success bool `json:"success"`
	Result  any  `json:"result,omitempty"`
	Error   any  `json:"error,omitempty"`
}

func IsDBError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) ||
		errors.Is(err, gorm.ErrForeignKeyViolated) ||
		errors.Is(err, gorm.ErrCheckConstraintViolated) {
		return true
	}
	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, driver.ErrBadConn) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var nerr net.Error
	if errors.As(err, &nerr) && (nerr.Timeout() || nerr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "sql:") || strings.HasPrefix(msg, "pq:") || strings.HasPrefix(msg, "pgx:") {
		return true
	}
	return false
}

func RespondOK(ctx *gin.Context, result any) {
	ctx.JSON(http.StatusOK, UnifiedResp{Success: true, Result: result})
}

func RespondErr(ctx *gin.Context, err error, logger *zap.Logger) {
	if IsDBError(err) {
		if logger != nil {
			logger.Error("db error", zap.String("path", ctx.FullPath()), zap.Error(err))
		}
		ctx.JSON(http.StatusServiceUnavailable, UnifiedResp{Success: false, Error: "系统错误"})
		return
	}
	ctx.JSON(http.StatusOK, UnifiedResp{Success: false, Error: err.Error()})
}

