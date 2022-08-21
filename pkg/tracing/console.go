package tracing

import (
	"go.uber.org/zap"
)

type ConsoleTracing struct {
	Log *zap.Logger
}

func (tr *ConsoleTracing) LogBody(req *TracingDetails) {
	log := tr.Log.With(zap.String("method", req.Method), zap.String("uri", req.Uri))
	if req.Body != "" {
		log.Info("req", zap.String("req body", req.Body))
	}
	if req.Resp != "" {
		log.Info("resp", zap.Int("status code", req.Status), zap.String("resp", req.Resp))
	}

}

func InitConsoleTracingService(log *zap.Logger) *ConsoleTracing {
	log.Debug("console tracing is enabled")
	return &ConsoleTracing{
		Log: log,
	}
}
