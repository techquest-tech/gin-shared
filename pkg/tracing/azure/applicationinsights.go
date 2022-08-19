package azure

import (
	"fmt"
	"os"

	"github.com/asaskevich/EventBus"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/event"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

type ApplicationInsightsClient struct {
	logger  *zap.Logger
	Key     string
	Role    string
	Version string
}

func init() {
	ginshared.ProvideController(func(logger *zap.Logger, bus EventBus.Bus) ginshared.DiController {
		client := &ApplicationInsightsClient{
			logger:  logger,
			Role:    core.AppName,
			Version: core.Version,
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

func (appins *ApplicationInsightsClient) getClient() appinsights.TelemetryClient {

	client := appinsights.NewTelemetryClient(appins.Key)
	if appins.Role != "" {
		client.Context().Tags.Cloud().SetRole(appins.Role)
	}
	if appins.Version != "" {
		client.Context().Tags.Application().SetVer(appins.Version)
	}
	return client
}

func (appins *ApplicationInsightsClient) ReportError(err error) {
	client := appins.getClient()
	trace := appinsights.NewTraceTelemetry(err.Error(), appinsights.Error)
	client.Track(trace)
	appins.logger.Debug("tracing error done", zap.Error(err))
}

func (appins *ApplicationInsightsClient) ReportTracing(tr *ginshared.TracingDetails) {
	client := appins.getClient()

	client.Context().Tags.Operation().SetName(fmt.Sprintf("%s %s", tr.Method, tr.Uri))

	t := appinsights.NewRequestTelemetry(
		tr.Method, tr.Uri, tr.Durtion, fmt.Sprintf("%d", tr.Status),
	)

	t.Source = tr.ClientIP
	t.Properties["user-agent"] = tr.UserAgent
	t.Properties["device"] = tr.Device
	if tr.Body != "" {
		t.Measurements["body-size"] = float64(len(tr.Body))
	}
	if tr.Resp != "" {
		t.Measurements["resp-size"] = float64(len(tr.Resp))
	}

	client.Track(t)
	appins.logger.Debug("submit tracing done.")
}
