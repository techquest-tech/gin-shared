package ginshared

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"strings"

	"github.com/gin-gonic/gin"
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
		buf, _ = ioutil.ReadAll(c.Request.Body)
	}
	c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
	return buf
}

func MD5(raw []byte) string {
	h := md5.New()
	h.Write(raw)
	signed := hex.EncodeToString(h.Sum(nil))
	return signed
}
