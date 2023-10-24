package ginshared

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type GeneralResp struct {
	Succ         bool
	ErrorCode    string
	ErrorMessage string
}

type ReportError struct {
	ReplyCode int
	logger    *zap.Logger
}

type ErrorCode interface {
	ErrorCode() string
}

func (handle *ReportError) RespErrorToClient(c *gin.Context, err interface{}) {
	handle.logger.Error("error found", zap.Any("error", err))
	errorResp := GeneralResp{
		Succ:         false,
		ErrorMessage: fmt.Sprintf("%+v", err),
	}
	if code, ok := err.(ErrorCode); ok {
		errorResp.ErrorCode = code.ErrorCode()
	}
	c.JSON(int(handle.ReplyCode), errorResp)
	c.Abort()
}

func (handle *ReportError) Middleware(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil {
			// print panic stack
			// buffer := make([]byte, 10240)
			// n := runtime.Stack(buffer, false)
			// fmt.Printf("recover from panic: \n %s", string(buffer[:n]))
			fmt.Printf("recover from panic: \n %s", debug.Stack())

			if _, ok := err.(error); !ok {
				err = fmt.Errorf("%v", err)
			}
			if core.Bus != nil {
				core.Bus.Publish(core.EventError, err)
			}
			handle.RespErrorToClient(c, err)
		}
	}()

	c.Next()
}

func NewErrorReport(replyCode int, logger *zap.Logger) gin.HandlerFunc {
	r := ReportError{
		ReplyCode: replyCode,
		logger:    logger,
	}
	return r.Middleware
}
