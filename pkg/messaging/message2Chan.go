package messaging

import (
	"context"
	"encoding/json"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type MessagingAdaptor[T any] struct {
	ChanAdaaptor *core.ChanAdaptor[T]
	Topic        string
}

func (r *MessagingAdaptor[T]) AsBridge(service MessagingService) {
	r.ChanAdaaptor.Subscripter("redis.bridge", func(data T) error {
		return service.Pub(context.Background(), r.Topic, data)
	})
}
func (r *MessagingAdaptor[T]) Adaptor(ctx context.Context, topic, consumer string, payload []byte) error {
	logger := zap.L()

	var tr T
	if err := json.Unmarshal(payload, &tr); err != nil {
		logger.Error("unexpected tracing details format", zap.ByteString("payload", payload), zap.Error(err))

		AbandonedChan <- map[string]any{
			"topic": topic,
			"raw":   payload,
			"error": err.Error(),
		}
		return nil
	}
	r.ChanAdaaptor.Push(tr)
	return nil
}
