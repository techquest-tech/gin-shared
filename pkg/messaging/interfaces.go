package messaging

import "context"

// MessagingService， default impl Redis streaming.
type MessagingService interface {
	Pub(ctx context.Context, topic string, payload any) error
	Sub(ctx context.Context, topic, consumer string, processor Processor) error
}

type Processor func(ctx context.Context, topic, consumer string, payload []byte) error
