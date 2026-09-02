package core

import (
	"sync/atomic"
	"testing"
	"time"
)

// compile-time check: the default implementation satisfies the Adaptor interface.
var _ Adaptor[int] = (*ChanAdaptor[int])(nil)

// compile-time checks: the chan adaptor also implements batched delivery and
// non-blocking push.
var _ BatchingAdaptor[int] = (*ChanAdaptor[int])(nil)
var _ TryPusher[int] = (*ChanAdaptor[int])(nil)

// TestChanAdaptorDropForSlowReceiverOnly verifies the default policy: a full
// receiver buffer only drops messages for THAT receiver, the other receivers
// still get every message.
func TestChanAdaptorDropForSlowReceiverOnly(t *testing.T) {
	ca := NewChanAdaptor[int](2)
	fast := ca.Sub("fast")
	slow := ca.Sub("slow")
	if fast == nil || slow == nil {
		t.Fatal("subscribe failed")
	}
	go ca.Start() // Start is a blocking fan-out loop, run it async like the event bus does.

	// slow is never drained: after its 2-buffer fills, messages must be dropped
	// for it, not for anyone else.

	var fastGot atomic.Int64
	done := make(chan struct{})
	go func() {
		for range fast {
			if fastGot.Add(1) == 6 {
				close(done)
			}
		}
	}()
	for i := 0; i < 6; i++ {
		ca.Push(i)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("fast receiver did not get all messages, got %d", fastGot.Load())
	}
	// the sender buffer lets Push return before the fan-out loop finishes
	// forwarding the last messages — wait until the loop has caught up.
	waitDrops(t, ca, "slow", 4, 3*time.Second)

	if got := ca.TotalDropped(); got < 4 {
		t.Fatalf("expected >= 4 drops for slow receiver, got %d", got)
	}
	if got := ca.DroppedCount("slow"); got < 4 {
		t.Fatalf("expected >= 4 drops for slow receiver, got %d", got)
	}
	if got := ca.DroppedCount("fast"); got != 0 {
		t.Fatalf("fast receiver must not drop, got %d", got)
	}
}

// waitDrops polls until the receiver accumulated at least want drops.
func waitDrops(t *testing.T, ca *ChanAdaptor[int], receiver string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for ca.DroppedCount(receiver) < uint64(want) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d drops on %q, got %d",
				want, receiver, ca.DroppedCount(receiver))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestChanAdaptorBlockTimeout verifies the "third option": with BlockTimeout
// set, a slow receiver gets a bounded grace period (no instant loss), and once
// proven slow its messages are dropped without waiting again, so the whole
// fan-out is not stalled by one stuck receiver.
func TestChanAdaptorBlockTimeout(t *testing.T) {
	const blockTimeout = 300 * time.Millisecond
	ca := NewChanAdaptor[int](2)
	ca.BlockTimeout = blockTimeout
	fast := ca.Sub("fast")
	slow := ca.Sub("slow")
	if fast == nil || slow == nil {
		t.Fatal("subscribe failed")
	}
	go ca.Start()

	var fastGot atomic.Int64
	done := make(chan struct{})
	go func() {
		for range fast {
			if fastGot.Add(1) == 6 {
				close(done)
			}
		}
	}()

	start := time.Now()
	for i := 0; i < 6; i++ {
		ca.Push(i)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("fast receiver did not get all messages, got %d", fastGot.Load())
	}
	// sender buffering: wait until the fan-out loop caught up with the slow
	// receiver's drops, then measure the total delivery time.
	waitDrops(t, ca, "slow", 4, 3*time.Second)
	elapsed := time.Since(start)

	if got := ca.DroppedCount("fast"); got != 0 {
		t.Fatalf("fast receiver must not drop, got %d", got)
	}

	// without the fast path, 4 full waits would take ~4*blockTimeout=1.2s;
	// with the fast path only the first wait happens (~0.3s).
	if elapsed > 3*blockTimeout {
		t.Fatalf("slow receiver stalled the fan-out: elapsed=%v (blockTimeout=%v)", elapsed, blockTimeout)
	}
	t.Logf("fan-out elapsed with slow receiver: %v", elapsed)
}

// TestAdaptorSubReturnsReceiveOnlyChannel ensures the interface contract:
// Sub returns a receive-only channel that is closed on Stop.
func TestAdaptorSubReturnsReceiveOnlyChannel(t *testing.T) {
	ca := NewChanAdaptor[string](4)
	ch := ca.Sub("one")
	go ca.Start()
	ca.Push("hello")

	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if v != "hello" {
			t.Fatalf("unexpected value: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received")
	}

	ca.Stop()
	// Stop closes the sender; the fan-out loop exits and closes receiver channels.
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, ok := <-ch
		if !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("channel not closed after Stop")
		}
	}
}

// collectBatches runs fn as a batch subscription and returns the received
// batches through a channel, so tests can assert on exact batch contents.
func collectBatches(t *testing.T, ca *ChanAdaptor[int], receiver string, batchSize int, interval time.Duration) chan []int {
	t.Helper()
	got := make(chan []int, 64)
	ca.SubscripterBatch(receiver, batchSize, interval, func(batch []int) error {
		got <- append([]int(nil), batch...)
		return nil
	})
	return got
}

// expectBatches waits until at least want batches arrived and returns them.
func expectBatches(t *testing.T, got chan []int, want int, timeout time.Duration) [][]int {
	t.Helper()
	batches := [][]int{}
	deadline := time.Now().Add(timeout)
	for len(batches) < want {
		select {
		case b := <-got:
			batches = append(batches, b)
		case <-time.After(time.Until(deadline)):
			t.Fatalf("timed out waiting for %d batches, got %d: %v", want, len(batches), batches)
		}
	}
	return batches
}

// TestChanAdaptorSubscripterBatchSize verifies size-triggered flushing: when
// batchSize messages accumulate, the handler is invoked once per batch with the
// exact slice, instead of once per message.
func TestChanAdaptorSubscripterBatchSize(t *testing.T) {
	ca := NewChanAdaptor[int](8)
	got := collectBatches(t, ca, "size", 3, 10*time.Second) // interval never fires
	go ca.Start()

	for i := 0; i < 6; i++ {
		ca.Push(i)
	}
	batches := expectBatches(t, got, 2, 3*time.Second)

	want := [][]int{{0, 1, 2}, {3, 4, 5}}
	for i, b := range batches {
		if len(b) != len(want[i]) {
			t.Fatalf("batch %d length = %d, want %d (%v)", i, len(b), len(want[i]), b)
		}
		for j := range want[i] {
			if b[j] != want[i][j] {
				t.Fatalf("batch %d = %v, want %v", i, b, want[i])
			}
		}
	}
}

// TestChanAdaptorSubscripterBatchInterval verifies interval-triggered flushing:
// a partial batch is delivered once flushInterval elapsed since its first
// message.
func TestChanAdaptorSubscripterBatchInterval(t *testing.T) {
	ca := NewChanAdaptor[int](8)
	got := collectBatches(t, ca, "interval", 100, 60*time.Millisecond) // size never reached
	go ca.Start()

	ca.Push(1)
	time.Sleep(20 * time.Millisecond)
	ca.Push(2)

	batches := expectBatches(t, got, 1, 2*time.Second)
	if len(batches[0]) != 2 || batches[0][0] != 1 || batches[0][1] != 2 {
		t.Fatalf("unexpected batch: %v", batches[0])
	}
}

// TestChanAdaptorSubscripterBatchFlushOnClose verifies the remaining partial
// batch is flushed when the adaptor stops, so no buffered message is lost on
// shutdown.
func TestChanAdaptorSubscripterBatchFlushOnClose(t *testing.T) {
	ca := NewChanAdaptor[int](8)
	got := collectBatches(t, ca, "close", 100, 10*time.Second)
	go ca.Start()

	ca.Push(7)
	ca.Push(8)
	ca.Stop()

	batches := expectBatches(t, got, 1, 3*time.Second)
	if len(batches[0]) != 2 || batches[0][0] != 7 || batches[0][1] != 8 {
		t.Fatalf("unexpected final batch: %v", batches[0])
	}
}

// TestChanAdaptorSubBatchPull verifies the pull variant: batches arrive on the
// returned channel and the channel is closed when the adaptor stops.
func TestChanAdaptorSubBatchPull(t *testing.T) {
	ca := NewChanAdaptor[int](8)
	pull := ca.SubBatch("pull", 3, 10*time.Second)
	go ca.Start()

	for i := 0; i < 3; i++ {
		ca.Push(i)
	}
	select {
	case b := <-pull:
		if len(b) != 3 || b[0] != 0 || b[2] != 2 {
			t.Fatalf("unexpected pull batch: %v", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no pull batch received")
	}

	ca.Stop()
	select {
	case _, ok := <-pull:
		if ok {
			t.Fatal("pull channel not closed after Stop")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pull channel not closed after Stop (timeout)")
	}
}

// TestChanAdaptorTryPush verifies the non-blocking push: once the sender
// buffer is full TryPush returns false without blocking, and true again once
// the fan-out drains.
func TestChanAdaptorTryPush(t *testing.T) {
	ca := NewChanAdaptor[int](1) // sender buffer capacity 1
	if !ca.TryPush(1) {
		t.Fatal("first try push should succeed")
	}
	if ca.TryPush(2) {
		t.Fatal("try push must fail while the sender buffer is full")
	}
	// start the fan-out with a receiver so the buffer drains
	ch := ca.Sub("drain")
	go ca.Start()
	select {
	case v, ok := <-ch:
		if !ok || v != 1 {
			t.Fatalf("unexpected drained value: %v, %v", v, ok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fan-out did not drain the buffered message")
	}
	// the sender buffer is empty again — but the next Push may race with the
	// fan-out loop; retry until TryPush succeeds to prove it is not stuck.
	deadline := time.Now().Add(2 * time.Second)
	for !ca.TryPush(3) {
		if time.Now().After(deadline) {
			t.Fatal("try push stuck failing after drain")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
