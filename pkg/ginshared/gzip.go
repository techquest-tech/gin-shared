// go:build gzip || all
package ginshared

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

type GzipEnabled struct {
	DefaultComponent
}

func (*GzipEnabled) OnEngineInited(r *gin.Engine) error {
	r.Use(gzip.Gzip(gzip.DefaultCompression))
	return nil
}

func init() {
	RegisterComponent(&GzipEnabled{})
}
