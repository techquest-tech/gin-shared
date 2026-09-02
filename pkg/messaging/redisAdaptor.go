//go:build !ram

package messaging

import (
	"context"
	"os"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

// RedisAdaptor implements core.Adaptor on top of Redis Streams (via
// MessagingService), so producers and consumers can live in different
// processes and a slow consumer only delays its own consumer group instead of
// blocking or dropping for everyone.
//
// Mapping to the interface:
//   - Push  -> XADD to the stream (no in-process buffer, producers never block
//     on a slow consumer).
//   - Sub / Subscripter -> one Redis consumer group per receiver. Messages are
//     ACKed only after the handler returns nil; a failing handler keeps the
//     message pending and it is redelivered by the pending-check schedule.
//
// It is compiled and usable only when the redis-backed messaging service is
// loaded (build tag !ram) — i.e. it is "loaded only when redis is loaded".
type RedisAdaptor[T any] struct {
	Topic   string
	Service MessagingService
	Buffer  int

	DedupEnabled bool
	DedupWindow  time.Duration

	ctx       context.Context
	cancel    context.CancelFunc
	started   atomic.Bool
	receivers sync.Map // receiver -> struct{}

	dedupOnce sync.Once
	deduper   *core.Deduper
}

// NewRedisAdaptor creates a redis-streaming adaptor for topic. service must be
// the redis-backed messaging service (nil panics). buf is the size of the pull
// channel returned by Sub. If buf <= 0 it defaults to 1.
func NewRedisAdaptor[T any](topic string, service MessagingService, buf int) *RedisAdaptor[T] {
	if service == nil {
		panic("messaging service is nil: redis adaptor requires the redis messaging service to be loaded")
	}
	if buf <= 0 {
		buf = 1
	}
	ra := &RedisAdaptor[T]{
		Topic:   topic,
		Service: service,
		Buffer:  buf,
	}
	ra.ctx, ra.cancel = context.WithCancel(context.Background())
	core.OnServiceStarted(ra.Start)
	core.OnServiceStopping(ra.Stop)
	return ra
}

// NewRedisAdaptorWithDedupChecking creates a redis adaptor with in-process
// duplicate checking enabled (same dedup semantics as the chan adaptor).
func NewRedisAdaptorWithDedupChecking[T any](topic string, service MessagingService, buf int, dedupWindow time.Duration) *RedisAdaptor[T] {
	ra := NewRedisAdaptor[T](topic, service, buf)
	ra.DedupEnabled = true
	ra.DedupWindow = dedupWindow
	return ra
}

func (ra *RedisAdaptor[T]) getLogger() *zap.Logger {
	var v T
	return zap.L().With(
		zap.String("adaptor", core.GetStructNameOnly(v)),
		zap.String("topic", ra.Topic),
	)
}

func (ra *RedisAdaptor[T]) Push(data T) {
	if ra.dedupSeen(data) {
		ra.getLogger().Info("duplicate data skipped")
		return
	}
	if err := ra.Service.Pub(context.Background(), ra.Topic, data); err != nil {
		// pub is best-effort: the stream is the buffer, so a failure here means
		// redis is unavailable — log and move on instead of blocking producers.
		ra.getLogger().Error("redis adaptor pub failed", zap.Error(err))
	}
}

// TryPush attempts to publish one message. Unlike the chan adaptor this is not
// an in-process non-blocking send (pub goes straight to redis), but it returns
// false when redis is unavailable so a caller can keep the message pending for
// redelivery instead of dropping it silently.
func (ra *RedisAdaptor[T]) TryPush(data T) bool {
	if ra.dedupSeen(data) {
		ra.getLogger().Info("duplicate data skipped")
		return true
	}
	if err := ra.Service.Pub(context.Background(), ra.Topic, data); err != nil {
		ra.getLogger().Error("redis adaptor pub failed", zap.Error(err))
		return false
	}
	return true
}

// dedupSeen reports whether the message is a duplicate and should be skipped.
func (ra *RedisAdaptor[T]) dedupSeen(data T) bool {
	if !ra.DedupEnabled || ra.DedupWindow <= 0 {
		return false
	}
	b, err := json.Marshal(data)
	if err != nil {
		ra.getLogger().Error("marshal data error", zap.Error(err))
		return false
	}
	ra.dedupOnce.Do(func() {
		ra.deduper = core.NewDeduper(ra.DedupWindow)
	})
	return ra.deduper.Seen(b)
}

// abandon pushes a payload into the global AbandonedChan without blocking:
// nothing in the workspace consumes that channel, so a blocking send here
// would hang the caller goroutine forever.
func abandon(logger *zap.Logger, payload map[string]any) {
	select {
	case AbandonedChan <- payload:
	default:
		logger.Warn("abandoned chan full or unread, dropped")
	}
}

// consumerGroupName 让每个进程（环境）的消费组唯一：同机同流的 uat/prd 各自独立
// 消费，互不抢消息（否则共享同一组时消息被两进程瓜分，导致 prd 落库缺一半）。
// 未设置 ENV 时保持原 receiver 名，兼容本地/历史行为。
func consumerGroupName(receiver string) string {
	if env := os.Getenv("ENV"); env != "" {
		return env + "." + receiver
	}
	return receiver
}

// startConsumer subscribes to the redis stream with a dedicated consumer group
// per receiver and hands every message to processor.
func (ra *RedisAdaptor[T]) startConsumer(receiver string, processor Processor) error {
	if ra.Service == nil {
		ra.getLogger().Error("messaging service not loaded, redis adaptor is not functional")
		return nil
	}
	group := consumerGroupName(receiver)
	err := ra.Service.Sub(ra.ctx, ra.Topic, group, processor)
	if err != nil {
		ra.getLogger().Error("redis adaptor subscribe failed",
			zap.String("receiver", receiver), zap.String("group", group), zap.Error(err))
		return err
	}
	ra.receivers.Store(receiver, struct{}{})
	ra.getLogger().Info("redis adaptor subscribed", zap.String("receiver", receiver), zap.String("group", group))
	return nil
}

func (ra *RedisAdaptor[T]) Sub(receiver string) <-chan T {
	if receiver == "" {
		receiver = randstr.Hex(16)
	}
	out := make(chan T, ra.Buffer)
	var closeOnce sync.Once
	closeOut := func() {
		closeOnce.Do(func() { close(out) })
	}
	go func() {
		<-ra.ctx.Done()
		closeOut()
	}()
	err := ra.startConsumer(receiver, func(ctx context.Context, topic, consumer string, payload []byte) error {
		var v T
		if err := json.Unmarshal(payload, &v); err != nil {
			ra.getLogger().Error("unexpected message format", zap.ByteString("payload", payload), zap.Error(err))
			AbandonedChan <- map[string]any{
				"topic": topic,
				"raw":   payload,
				"error": err.Error(),
			}
			return nil
		}
		select {
		case out <- v:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		closeOut()
	}
	return out
}

func (ra *RedisAdaptor[T]) Subscripter(receiver string, fn core.Handler[T]) {
	l := ra.getLogger().With(zap.String("receiver", receiver))
	if fn == nil {
		l.Warn("handler is nil")
		return
	}
	err := ra.startConsumer(receiver, func(ctx context.Context, topic, consumer string, payload []byte) error {
		var v T
		if err := json.Unmarshal(payload, &v); err != nil {
			ra.getLogger().Error("unexpected message format", zap.ByteString("payload", payload), zap.Error(err))
			abandon(ra.getLogger(), map[string]any{
				"topic": topic,
				"raw":   payload,
				"error": err.Error(),
			})
			return nil
		}
		if err := fn(v); err != nil {
			// keep the message pending so the pending-check redelivers it later.
			l.Error("handler error", zap.Error(err))
			return err
		}
		return nil
	})
	if err != nil {
		l.Error("subscribe failed", zap.Error(err))
	}
}

// startBatchConsumer subscribes to the redis stream with a dedicated consumer
// group per receiver; a batch of messages is handed to processor in one call
// and only acknowledged after the processor returns nil.
func (ra *RedisAdaptor[T]) startBatchConsumer(receiver string, batchSize int, flushInterval time.Duration, processor BatchProcessor) error {
	if ra.Service == nil {
		ra.getLogger().Error("messaging service not loaded, redis adaptor is not functional")
		return nil
	}
	err := ra.Service.SubBatch(ra.ctx, ra.Topic, consumerGroupName(receiver), batchSize, flushInterval, processor)
	if err != nil {
		ra.getLogger().Error("redis adaptor batch subscribe failed",
			zap.String("receiver", receiver), zap.Error(err))
		return err
	}
	ra.receivers.Store(receiver, struct{}{})
	ra.getLogger().Info("redis adaptor batch subscribed", zap.String("receiver", receiver))
	return nil
}

// SubscripterBatch registers a batch consumer: messages are accumulated by the
// redis layer into groups and fn is called once per group. A handler error
// keeps the whole group pending so it is redelivered — a downstream write
// failure (e.g. loki queue full, parquet write error) no longer silently loses
// data that was already read out of the stream.
func (ra *RedisAdaptor[T]) SubscripterBatch(receiver string, batchSize int, flushInterval time.Duration, fn core.BatchHandler[T]) {
	l := ra.getLogger().With(zap.String("receiver", receiver))
	if fn == nil {
		l.Warn("handler is nil")
		return
	}
	err := ra.startBatchConsumer(receiver, batchSize, flushInterval, func(ctx context.Context, topic, consumer string, payloads [][]byte) error {
		msgs := make([]T, 0, len(payloads))
		for _, payload := range payloads {
			var v T
			if err := json.Unmarshal(payload, &v); err != nil {
				ra.getLogger().Error("unexpected message format", zap.ByteString("payload", payload), zap.Error(err))
				abandon(ra.getLogger(), map[string]any{
					"topic": topic,
					"raw":   payload,
					"error": err.Error(),
				})
				continue
			}
			msgs = append(msgs, v)
		}
		if len(msgs) == 0 {
			return nil
		}
		if err := fn(msgs); err != nil {
			l.Error("batch handler error", zap.Int("len", len(msgs)), zap.Error(err))
			return err
		}
		return nil
	})
	if err != nil {
		l.Error("batch subscribe failed", zap.Error(err))
	}
}

// SubBatch registers a named receiver and returns its pull channel of batches
// ([]T), closed when the adaptor stops.
func (ra *RedisAdaptor[T]) SubBatch(receiver string, batchSize int, flushInterval time.Duration) <-chan []T {
	if receiver == "" {
		receiver = randstr.Hex(16)
	}
	out := make(chan []T, ra.Buffer)
	var closeOnce sync.Once
	closeOut := func() {
		closeOnce.Do(func() { close(out) })
	}
	go func() {
		<-ra.ctx.Done()
		closeOut()
	}()
	err := ra.startBatchConsumer(receiver, batchSize, flushInterval, func(ctx context.Context, topic, consumer string, payloads [][]byte) error {
		msgs := make([]T, 0, len(payloads))
		for _, payload := range payloads {
			var v T
			if err := json.Unmarshal(payload, &v); err != nil {
				ra.getLogger().Error("unexpected message format", zap.ByteString("payload", payload), zap.Error(err))
				abandon(ra.getLogger(), map[string]any{
					"topic": topic,
					"raw":   payload,
					"error": err.Error(),
				})
				continue
			}
			msgs = append(msgs, v)
		}
		if len(msgs) == 0 {
			return nil
		}
		select {
		case out <- msgs:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		closeOut()
	}
	return out
}

func (ra *RedisAdaptor[T]) Start() {
	if ra.started.Swap(true) {
		return
	}
	ra.getLogger().Info("redis adaptor started")
}

func (ra *RedisAdaptor[T]) Stop() {
	ra.getLogger().Info("redis adaptor stopping")
	ra.cancel()
}

func (ra *RedisAdaptor[T]) Receivers() []string {
	out := make([]string, 0, 8)
	ra.receivers.Range(func(k, _ any) bool {
		out = append(out, k.(string))
		return true
	})
	return out
}

// compile-time check: *RedisAdaptor[T] satisfies core.Adaptor[T].
var _ core.Adaptor[core.ErrorReport] = (*RedisAdaptor[core.ErrorReport])(nil)
