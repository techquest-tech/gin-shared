//go:build gzip || all

package ginshared

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

type GzipEnabled struct {
	DefaultComponent
}

func (*GzipEnabled) OnEngineInited(r *gin.Engine) error {
	r.Use(gzip.Gzip(gzip.BestCompression, gzip.WithExcludedExtensions([]string{".png", ".jpg", ".gif", ".mp4", ".zip", ".pdf"})))
	return nil
}

func init() {
	RegisterComponent(&GzipEnabled{})
}
