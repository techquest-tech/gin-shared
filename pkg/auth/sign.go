package auth

import (
	"fmt"
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
			KeyTimestamp: "timestamp",
			KeySign:      "sign",
			KeyApp:       "app",
			MaxDuration:  30 * time.Minute,
		}
		settings := viper.Sub("auth.sign")

		if settings != nil {
			settings.Unmarshal(&ss)
			return ss, nil
		} else {
			logging.Debug("no settings for sign auth")
			return nil, nil
		}
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
	reqParsed := time.UnixMilli(mm)
	duration := time.Since(reqParsed)
	if duration < 0 {
		duration = -1 * duration
	}

	if duration > ss.MaxDuration {
		ss.logger.Warn("timestamp is out of max duration", zap.Duration("duration", duration),
			zap.Time("parsedValue", reqParsed),
			zap.String("headerValue", reqTime))
		c.JSON(http.StatusBadRequest, fmt.Sprintf("请求已超过最大允许值(%s), 实时差异 %s", ss.MaxDuration, duration))
		c.Abort()
		return
	}

	ss.logger.Debug("check timestamp done", zap.Duration("duration", duration))

	c.Next()

}

func (ss *SignService) Sign(c *gin.Context) {
	// buf := bytes.Buffer{}

	appID := c.GetHeader(ss.KeyApp)
	ts := c.GetHeader(ss.KeyTimestamp)
	// buf.WriteString("app=")
	// buf.WriteString(appID)
	secret, ok := ss.Secrets[appID]
	if !ok {
		ss.logger.Error("invalid appID", zap.String("reqID", appID))
		c.JSON(http.StatusUnauthorized, fmt.Sprintf("非法%s %s", ss.KeyApp, appID))
		c.Abort()
		return
	}
	// buf.WriteString("&secret=")
	// buf.WriteString(secret)
	// buf.WriteString("&timestamp=")
	// buf.WriteString(c.GetHeader(ss.KeyTimestamp))

	// buf.WriteString("&body=")
	// buf.Write(ginshared.CloneRequestBody(c))

	signed, err := SignRequest(appID, ts, secret, ginshared.CloneRequestBody(c))
	if err != nil {
		ss.logger.Error("signed failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, "验证签名失败")
		c.Abort()
		return
	}

	reqSigned := c.GetHeader(ss.KeySign)
	if signed != reqSigned {
		ss.logger.Error("sign validation failed.", zap.String("req", reqSigned), zap.String("signed", signed))
		c.JSON(http.StatusUnauthorized, "验证签名失败")
		c.Abort()
		return
	}
	ss.logger.Debug("signed check passed.", zap.String("signed", signed))

	c.Next()
}
