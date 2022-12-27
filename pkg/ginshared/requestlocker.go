package ginshared

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLocker(params ...string) gin.HandlerFunc {
	cache := &sync.Map{}
	return func(ctx *gin.Context) {
		key := ""
		for _, item := range params {
			key = key + ctx.Param(item)
		}
		if key == "" {
			key = ctx.Request.RequestURI
		}
		_, ok := cache.LoadOrStore(key, true)
		defer cache.Delete(key)
		if ok {
			zap.L().Warn("another request is processing, this request will be ignored.")
			ctx.JSON(http.StatusTooManyRequests, gin.H{
				"error": "another request is processing",
			})
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}
