package messaging

import (
	"context"
	"time"
)

// MessagingService, default impl Redis streaming.
type MessagingService interface {
	Pub(ctx context.Context, topic string, payload any) error
	Sub(ctx context.Context, topic, consumer string, processor Processor) error
	// SubBatch registers a consumer that receives messages in batches: the
	// backend hands a group of payloads to processor at once and only
	// acknowledges the whole group after processor returns nil. A failing
	// batch stays pending and is redelivered by the pending-check schedule —
	// no silent loss between "read from the stream" and "written downstream".
	// batchSize is the max messages per read (<=0 = all available);
	// flushInterval is how long the consumer waits for more messages before
	// handing over the partial batch (<=0 falls back to 1s).
	SubBatch(ctx context.Context, topic, consumer string, batchSize int, flushInterval time.Duration, processor BatchProcessor) error
}

type Processor func(ctx context.Context, topic, consumer string, payload []byte) error

// BatchProcessor handles a batch of payloads read from the stream in one call.
// Returning an error keeps the whole batch unacknowledged (pending) so it can
// be redelivered later.
type BatchProcessor func(ctx context.Context, topic, consumer string, payloads [][]byte) error
