package azure

import (
	"fmt"
	"os"
	"time"

	"github.com/asaskevich/EventBus"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/tracing"
	"go.uber.org/zap"
)

type ApplicationInsightsClient struct {
	logger *zap.Logger
	Key    string
}

func init() {
	ginshared.ProvideController(func(logger *zap.Logger, bus EventBus.Bus) ginshared.DiController {
		client := &ApplicationInsightsClient{
			logger: logger,
		}
		settings := viper.Sub("tracing.azure")
		if settings != nil {
			settings.Unmarshal(client)
		}
		if keyFromenv := os.Getenv("APPINSIGHTS_INSTRUMENTATIONKEY"); keyFromenv != "" {
			client.Key = keyFromenv
			logger.Info("read application insights key from ENV")
		}

		if client.Key == "" {
			logger.Warn("no application insights key provided, tracing function disabled.")
			return nil
		}

		bus.SubscribeAsync(event.EventError, client.ReportError, false)
		bus.SubscribeAsync(event.EventTracing, client.ReportTracing, false)
		logger.Info("event subscribed for application insights")
		return client
	})
}

func (appins *ApplicationInsightsClient) ReportError(err error) {
	client := appinsights.NewTelemetryClient(appins.Key)
	trace := appinsights.NewTraceTelemetry(err.Error(), appinsights.Error)
	client.Track(trace)
	appins.logger.Debug("tracing error done", zap.Error(err))
}

func (appins *ApplicationInsightsClient) ReportTracing(tr *tracing.TracingDetails) {
	client := appinsights.NewTelemetryClient(appins.Key)
	t := appinsights.NewRequestTelemetry(
		tr.Method, tr.Uri, tr.Durtion, fmt.Sprintf("%d", tr.Status),
	)
	t.Timestamp = time.Now()
	client.Track(t)
	appins.logger.Debug("submit tracing done.")
}
