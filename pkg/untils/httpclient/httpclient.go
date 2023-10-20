package httpclient

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/avast/retry-go"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"github.com/techquest-tech/monitor"
	"go.uber.org/zap"
)

func RequestWithRetry(req *http.Request, result interface{}, body ...string) error {
	log := zap.L().With(zap.String("service", "clientWithRetry"))

	client := &http.Client{}
	err := retry.Do(func() error {
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		cached, err := io.ReadAll(resp.Body)

		if event.Bus != nil && len(body) > 0 {
			fulllogging := &monitor.TracingDetails{
				Optionname: req.URL.String(),
				Uri:        req.URL.String(),
				Method:     req.Method,
				Body:       body[0],
				Durtion:    time.Since(start),
				Status:     resp.StatusCode,
				Resp:       string(cached),
				// App:        core.AppName,
				// Version:    core.Version,
			}
			event.Bus.Publish(event.EventTracing, fulllogging)
		}

		if err != nil {
			return err
		}
		log.Info("agent replied", zap.Int("statusCode", resp.StatusCode), zap.String("status", resp.Status))
		log.Debug("resp body", zap.String("resp", string(cached)))

		err = json.Unmarshal(cached, result)
		if err != nil {
			log.Error("decode resp to object failed.", zap.Error(err))
			return err
		}
		log.Info("request upstream done.", zap.Any("result", result))

		return nil
	})
	return err
}
