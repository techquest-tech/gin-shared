package ginshared

import (
	"bytes"
	"io"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"go.uber.org/zap"
)

const (
	KeyTracingID = "tracingID"
)

// FullRequestDetails

type TracingDetails struct {
	Origin    string
	Uri       string
	Method    string
	Body      string
	Durtion   time.Duration
	Status    int
	TargetID  uint
	Resp      string
	ClientIP  string
	UserAgent string
	Device    string
	// Props     map[string]interface{}
}

type RespLogging struct {
	gin.ResponseWriter
	cache *bytes.Buffer
}

func (w RespLogging) Write(b []byte) (int, error) {
	w.cache.Write(b)
	return w.ResponseWriter.Write(b)
}

type TracingRequestService struct {
	Bus     EventBus.Bus
	Log     *zap.Logger
	Enabled bool
	Request bool
	Resp    bool
}

func init() {
	Provide(func(bus EventBus.Bus, logger *zap.Logger) *TracingRequestService {
		sr := &TracingRequestService{
			Bus: bus,
			Log: logger,
		}

		settings := viper.Sub("tracing")
		if settings != nil {
			settings.Unmarshal(sr)
		}

		if sr.Request || sr.Resp {
			bus.SubscribeAsync(event.EventTracing, sr.LogBody, false)
		}
		return sr
	})
}

func (tr *TracingRequestService) LogBody(req *TracingDetails) {
	log := tr.Log.With(zap.String("method", req.Method), zap.String("uri", req.Uri))
	log.Info("req", zap.String("req body", req.Body))
	log.Info("resp", zap.Int("status code", req.Status), zap.String("resp", req.Resp))
}

func (tr *TracingRequestService) LogfullRequestDetails(c *gin.Context) {
	start := time.Now()
	reqcache := make([]byte, 0)

	if tr.Request {
		if c.Request.Body != nil {
			reqcache, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqcache))
		}
	}

	// respcache := make([]byte, 0)
	writer := &RespLogging{
		cache:          bytes.NewBuffer([]byte{}),
		ResponseWriter: c.Writer,
	}

	if tr.Resp {
		c.Writer = writer
	}

	uri := c.Request.RequestURI

	c.Next()

	dur := time.Since(start)

	status := c.Writer.Status()
	rawID := c.GetUint(KeyTracingID)

	respcache := writer.cache.Bytes()

	fullLogging := &TracingDetails{
		Origin:    c.Request.Header.Get("Origin"),
		Uri:       uri,
		Method:    c.Request.Method,
		Body:      string(reqcache),
		Durtion:   dur,
		Status:    status,
		TargetID:  rawID,
		Resp:      string(respcache),
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Device:    c.GetHeader("deviceID"),
	}

	tr.Bus.Publish(event.EventTracing, fullLogging)
}
