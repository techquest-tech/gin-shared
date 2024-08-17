package ginshared

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/techquest-tech/gin-shared/pkg/core"
)

func DropDuplicated(raw []string) []string {
	filterd := make([]string, 0)
	set := make(map[string]bool)
	for _, item := range raw {
		if set[item] {
			continue
		}
		item = strings.TrimSpace(item)
		set[item] = true
		filterd = append(filterd, item)
	}
	return filterd
}

func CloneRequestBody(c *gin.Context) []byte {
	buf := make([]byte, 0)
	if c.Request.Body != nil {
		buf, _ = io.ReadAll(c.Request.Body)
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(buf))
	return buf
}

var MD5 = core.MD5
