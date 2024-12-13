package ginshared

import (
	"fmt"
	"runtime"

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
	// logger    *zap.Logger
}

type ErrorCode interface {
	ErrorCode() string
}

func (handle *ReportError) RespErrorToClient(c *gin.Context, err interface{}) {
	zap.L().Error("error found", zap.Any("error", err))
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
			buffer := make([]byte, 10240)
			n := runtime.Stack(buffer, false)
			fmt.Printf("gin recover from panic: \n %s", string(buffer[:n]))
			// fmt.Printf("recover from panic: \n %s", debug.Stack())
			var e error
			e, ok := err.(error)
			if !ok {
				e = fmt.Errorf("%v", err)
			}
			core.ErrorAdaptor.Push(e)
			// if core.Bus != nil {
			// 	core.Bus.Publish(core.EventError, err)
			// }
			handle.RespErrorToClient(c, err)
		}
	}()

	c.Next()
}
func (h *ReportError) Priority() int { return 10 }

func (h *ReportError) OnEngineInited(r *gin.Engine) error {
	zap.L().Info("register error handler")
	r.Use(h.Middleware)
	return nil
}

func init() {
	RegisterComponent(&ReportError{ReplyCode: 500})
}

// deprecated
func NewErrorReport(replyCode int, logger *zap.Logger) gin.HandlerFunc {
	// r := ReportError{
	// 	ReplyCode: replyCode,
	// 	logger:    logger,
	// }
	// return r.Middleware
	return func(ctx *gin.Context) { ctx.Next() }
}
