//go:build !ram

package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

// fakeMessagingService captures the BatchProcessor registered by a batch
// subscription so tests can drive it directly (no redis needed for the
// adaptor-layer semantics).
type fakeMessagingService struct {
	batchProcessor BatchProcessor
	subCalls       atomic.Int64
}

func (f *fakeMessagingService) Pub(ctx context.Context, topic string, payload any) error { return nil }
func (f *fakeMessagingService) Sub(ctx context.Context, topic, consumer string, processor Processor) error {
	return nil
}
func (f *fakeMessagingService) SubBatch(ctx context.Context, topic, consumer string, batchSize int, flushInterval time.Duration, processor BatchProcessor) error {
	f.batchProcessor = processor
	f.subCalls.Add(1)
	return nil
}

// TestRedisAdaptorSubscripterBatch verifies the redis adaptor batch path: the
// handler receives the unmarshalled batch in one call, a handler error
// propagates back (so the whole batch stays pending in the stream), and
// unmarshal failures are abandoned without blocking.
func TestRedisAdaptorSubscripterBatch(t *testing.T) {
	fake := &fakeMessagingService{}
	ra := NewRedisAdaptor[string]("test.topic", fake, 4)

	var batches [][]string
	ra.SubscripterBatch("receiver", 10, time.Second, func(batch []string) error {
		batches = append(batches, append([]string(nil), batch...))
		return nil
	})
	assert.Equal(t, int64(1), fake.subCalls.Load(), "batch consumer must be registered")

	// drive the captured batch processor with two payloads
	err := fake.batchProcessor(context.Background(), "test.topic", "receiver",
		[][]byte{[]byte(`"a"`), []byte(`"b"`)})
	assert.NoError(t, err)
	assert.Equal(t, [][]string{{"a", "b"}}, batches, "handler must get one batch with both messages")

	// a handler error must propagate so the batch stays pending
	fakeErr := &fakeMessagingService{}
	raErr := NewRedisAdaptor[string]("test.topic", fakeErr, 4)
	raErr.SubscripterBatch("receiver", 10, time.Second, func(batch []string) error {
		return errors.New("downstream write failed")
	})
	err = fakeErr.batchProcessor(context.Background(), "test.topic", "receiver",
		[][]byte{[]byte(`"x"`)})
	assert.Error(t, err, "handler error must propagate to the redis consumer")

	// unmarshal failure: the bad payload is abandoned, the good one still delivered
	fakeMix := &fakeMessagingService{}
	raMix := NewRedisAdaptor[string]("test.topic", fakeMix, 4)
	var goodBatches [][]string
	raMix.SubscripterBatch("receiver", 10, time.Second, func(batch []string) error {
		goodBatches = append(goodBatches, append([]string(nil), batch...))
		return nil
	})
	err = fakeMix.batchProcessor(context.Background(), "test.topic", "receiver",
		[][]byte{[]byte("not-json"), []byte(`"ok"`)})
	assert.NoError(t, err)
	assert.Equal(t, [][]string{{"ok"}}, goodBatches, "valid messages must survive invalid ones")
}

// testRedisClient returns a client to the local redis, skipping the test when
// no redis is reachable (CI / offline runs).
func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		t.Skipf("local redis not available, skip: %v", err)
	}
	return client
}

// TestMessagingSubBatchAckAfterBatch verifies the core redis semantics behind
// the batch mode: messages read from the stream are acknowledged ONLY after
// the batch processor returns nil. A failing processor leaves the whole batch
// pending (no silent loss between "read from redis" and "written downstream").
func TestMessagingSubBatchAckAfterBatch(t *testing.T) {
	client := testRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	svc := &DefaultMessgingService{
		Client:   client,
		Logger:   zap.L(),
		Settings: map[string]int64{},
	}

	t.Run("failed batch stays pending", func(t *testing.T) {
		topic := fmt.Sprintf("test.subbatch.fail.%d", time.Now().UnixNano())
		group := "batch-test"
		assert.NoError(t, svc.checkAndCreate(ctx, topic, group))
		for i := 0; i < 3; i++ {
			assert.NoError(t, svc.Pub(ctx, topic, map[string]any{"i": i}))
		}

		calls := atomic.Int64{}
		err := svc.SubBatch(ctx, topic, group, 10, 100*time.Millisecond,
			func(ctx context.Context, topic, consumer string, payloads [][]byte) error {
				calls.Add(1)
				return errors.New("injected downstream failure")
			})
		assert.NoError(t, err)

		// the consumer must read the batch, fail, and leave all 3 pending
		waitFor(t, 3*time.Second, func() bool {
			p, _ := client.XPending(ctx, topic, group).Result()
			return calls.Load() >= 1 && p.Count >= 3
		}, "batch must stay pending after processor failure")
	})

	t.Run("successful batch is acked", func(t *testing.T) {
		topic := fmt.Sprintf("test.subbatch.ok.%d", time.Now().UnixNano())
		group := "batch-test"
		assert.NoError(t, svc.checkAndCreate(ctx, topic, group))
		for i := 0; i < 3; i++ {
			assert.NoError(t, svc.Pub(ctx, topic, map[string]any{"i": i}))
		}

		var got int
		err := svc.SubBatch(ctx, topic, group, 10, 100*time.Millisecond,
			func(ctx context.Context, topic, consumer string, payloads [][]byte) error {
				got = len(payloads)
				return nil
			})
		assert.NoError(t, err)

		waitFor(t, 3*time.Second, func() bool {
			p, _ := client.XPending(ctx, topic, group).Result()
			return got == 3 && p.Count == 0
		}, "successful batch must be acknowledged")
	})
}

// waitFor polls cond until it returns true or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}

// TestMessagingSubBatchCoalescesWithinFlushInterval verifies the batch consumer
// keeps reading for up to flushInterval after the first message, so messages
// arriving inside the window are handed to the processor as ONE batch. This
// turns many tiny flushes into a single multi-row write downstream (e.g. the
// MyDuck single-writer path where every INSERT creates a temp table), without
// changing ack semantics: ack still happens only after the processor returns.
func TestMessagingSubBatchCoalescesWithinFlushInterval(t *testing.T) {
	client := testRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	svc := &DefaultMessgingService{
		Client:   client,
		Logger:   zap.L(),
		Settings: map[string]int64{},
	}
	topic := fmt.Sprintf("test.subbatch.coalesce.%d", time.Now().UnixNano())
	group := "batch-test"

	// 先落 2 条，保证消费者首读即有数据（首读不会因窗口空闲而等待）。
	assert.NoError(t, svc.checkAndCreate(ctx, topic, group))
	for i := 0; i < 2; i++ {
		assert.NoError(t, svc.Pub(ctx, topic, map[string]any{"i": i}))
	}

	var (
		mu    sync.Mutex
		calls []int
	)
	err := svc.SubBatch(ctx, topic, group, 10, 600*time.Millisecond,
		func(ctx context.Context, topic, consumer string, payloads [][]byte) error {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, len(payloads))
			return nil
		})
	assert.NoError(t, err)

	// 首读后 ~150ms（仍在 600ms 聚合窗口内）再补 2 条：应并入同一批。
	time.Sleep(150 * time.Millisecond)
	for i := 2; i < 4; i++ {
		assert.NoError(t, svc.Pub(ctx, topic, map[string]any{"i": i}))
	}

	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(calls) == 1 && calls[0] == 4
	}, "messages arriving inside the flush window must be coalesced into one batch")
}

// compile-time checks for the new optional capabilities.
var _ core.BatchingAdaptor[string] = (*RedisAdaptor[string])(nil)
var _ core.TryPusher[string] = (*RedisAdaptor[string])(nil)
