package tracing

import (
	"bytes"
	"io"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

const (
	KeyTracingID = "tracingID"
)

// FullRequestDetails

type TracingDetails struct {
	Origin   string
	Uri      string
	Method   string
	Body     string
	Durtion  time.Duration
	Status   int
	TargetID uint
	Resp     string
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
	ginshared.Provide(func(bus EventBus.Bus, logger *zap.Logger) event.EventComponent {
		sr := &TracingRequestService{
			Bus: bus,
			Log: logger,
		}

		settings := viper.Sub("tracing")
		if settings != nil {
			settings.Unmarshal(sr)
		}
		if sr.Enabled {
			bus.Subscribe(event.EventInit, sr.Enable)
			logger.Info("tracing is enabled.")
		}
		return sr
	}, event.EventOptions)
}

func (tr *TracingRequestService) Enable(route *gin.Engine) {
	route.Use(tr.LogfullRequestDetails)
	tr.Log.Info("received init event.")
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

	respcache := make([]byte, 0)

	if tr.Resp {
		c.Writer = &RespLogging{
			cache:          bytes.NewBuffer(respcache),
			ResponseWriter: c.Writer,
		}
	}

	uri := c.Request.RequestURI

	c.Next()

	dur := time.Since(start)

	status := c.Writer.Status()
	rawID := c.GetUint(KeyTracingID)

	fullLogging := TracingDetails{
		Origin:   c.Request.Header.Get("Origin"),
		Uri:      uri,
		Method:   c.Request.Method,
		Body:     string(reqcache),
		Durtion:  dur,
		Status:   status,
		TargetID: rawID,
		Resp:     string(respcache),
	}

	tr.Bus.Publish(event.EventTracing, fullLogging)
}
