package auth

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func init() {
	ginshared.GetContainer().Provide(func(logging *zap.Logger) (*SignService, error) {
		ss := &SignService{
			logger:       logging,
			KeyTimestamp: "time_stamp",
			KeySign:      "sign",
			KeyApp:       "app_id",
			MaxDuration:  30 * time.Minute,
		}
		settings := viper.Sub("auth.sign")
		if settings != nil {
			settings.Unmarshal(&ss)
		}

		return ss, nil
	})
}

type SignService struct {
	logger       *zap.Logger
	MaxDuration  time.Duration
	KeyTimestamp string
	KeySign      string
	KeyApp       string
	Secrets      map[string]string
}

func (ss *SignService) CheckMaxDuration(c *gin.Context) {
	reqTime := c.GetHeader(ss.KeyTimestamp)
	if reqTime == "" {
		ss.logger.Warn("header timestamp is missed. request rejected.")
		c.JSON(http.StatusBadRequest, fmt.Sprintf("header %s is missed", ss.KeyTimestamp))
		c.Abort()
		return
	}

	mm, _ := strconv.ParseInt(reqTime, 10, 64)
	duration := time.Since(time.UnixMilli(mm))

	if duration > ss.MaxDuration {
		ss.logger.Warn("timestamp is out of max duration", zap.Duration("duration", duration))
		c.JSON(http.StatusBadRequest, fmt.Sprintf("请求已超过最大允许值(%s), 实时差异 %s", ss.MaxDuration, duration))
		c.Abort()
		return
	}

	c.Next()

}

func (ss *SignService) Sign(c *gin.Context) {
	buf := bytes.Buffer{}
	buf.WriteString("timestamp=")
	buf.WriteString(c.GetHeader(ss.KeyTimestamp))
	buf.WriteString("&body=")
	buf.Write(CloneRequestBody(c))

	appID := c.GetHeader(ss.KeyApp)
	secret, ok := ss.Secrets[appID]
	if !ok {
		ss.logger.Error("invalid appID", zap.String("reqID", appID))
		c.JSON(http.StatusUnauthorized, fmt.Sprintf("非法%s %s", ss.KeyApp, appID))
		c.Abort()
		return
	}
	buf.WriteString("&secret=")
	buf.WriteString(secret)

	signed := MD5(buf.Bytes())

	reqSigned := c.GetHeader(ss.KeySign)
	if signed != reqSigned {
		ss.logger.Error("sign validation failed.", zap.String("req", reqSigned), zap.String("signed", signed))
		c.JSON(http.StatusUnauthorized, "验证签名失败")
		c.Abort()
		return
	}
	ss.logger.Debug("signed check passed.")
	c.Next()
}

func MD5(raw []byte) string {
	h := md5.New()
	h.Write(raw)
	signed := hex.EncodeToString(h.Sum(nil))
	return signed
}

func CloneRequestBody(c *gin.Context) []byte {
	buf := make([]byte, 0)
	if c.Request.Body != nil {
		buf, _ = ioutil.ReadAll(c.Request.Body)
	}
	c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
	return buf
}
