//go:build ram

package messaging

import (
	"context"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type MessagingDisabled struct {
}

// it's dummy messaging,
func (m *MessagingDisabled) Pub(ctx context.Context, topic string, payload any) error {
	return nil
}
func (m *MessagingDisabled) Sub(ctx context.Context, topic, consumer string, processor Processor) error {
	zap.L().Info("messaging is disabled.")
	return nil
}

func init() {
	core.Provide(func() MessagingService {
		return &MessagingDisabled{}
	})
}
