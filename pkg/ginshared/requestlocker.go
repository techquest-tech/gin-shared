package ginshared

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

func RequestLocker(params ...string) gin.HandlerFunc {
	var ll locker.Locker
	core.GetContainer().Invoke(func(l locker.Locker) {
		ll = l
	})
	return func(ctx *gin.Context) {
		key := ""
		for _, item := range params {
			key = key + ctx.Param(item)
		}
		if key == "" {
			key = ctx.Request.RequestURI
		}
		rr, err := ll.LockWithtimeout(ctx, key, time.Millisecond*100)
		if err != nil {
			zap.L().Warn("another request is processing, this request will be ignored.")
			ctx.JSON(http.StatusTooManyRequests, gin.H{
				"error": "another request is processing",
			})
			ctx.Abort()
			return
		}
		defer rr(ctx)

		ctx.Next()
	}
}
