package core

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/samber/lo"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

// Handler processes one message. Returning an error marks the message as
// failed; how the failure is handled depends on the adaptor implementation
// (chan adaptor just logs it, the redis adaptor keeps the message pending so
// it can be redelivered).
type Handler[T any] func(data T) error

// BatchHandler processes a batch of messages delivered together. Returning an
// error marks the WHOLE batch as failed: the chan adaptor logs it (and the
// batch is dropped from memory), while the redis adaptor keeps the batch
// pending so it can be redelivered — no silent loss on the redis path.
type BatchHandler[T any] func(data []T) error

// DefaultBatchSize / DefaultBatchInterval are the flush thresholds used by
// batch subscriptions (SubBatch / SubscripterBatch) when the caller passes 0.
// They match the defaults the loki and parquet consumers already use.
const (
	DefaultBatchSize     = 200
	DefaultBatchInterval = 30 * time.Second
)

// BatchingAdaptor is implemented by adaptors that additionally support batched
// delivery: messages are accumulated per receiver and handed over as slices
// (flushed when batchSize is reached or flushInterval elapsed since the first
// message of the batch), instead of one message at a time. The base Adaptor
// interface is intentionally left untouched so existing consumers keep working.
type BatchingAdaptor[T any] interface {
	Adaptor[T]
	// SubBatch registers a named receiver and returns its pull channel of
	// batches. The channel is closed when the adaptor stops.
	SubBatch(receiver string, batchSize int, flushInterval time.Duration) <-chan []T
	// SubscripterBatch registers a named receiver driven by a batch handler
	// goroutine (one handler call per batch, not per message).
	SubscripterBatch(receiver string, batchSize int, flushInterval time.Duration, fn BatchHandler[T])
}

// TryPusher is implemented by adaptors that support a non-blocking Push so a
// caller can decide what to do when the adaptor is saturated — e.g. the redis
// consumer keeps the message pending for redelivery instead of blocking the
// whole consumer loop (which used to cascade into queue delay).
type TryPusher[T any] interface {
	// TryPush attempts to publish one message without blocking. It returns
	// false when the adaptor is saturated and the message was NOT accepted.
	TryPush(data T) bool
}

// Adaptor is the abstraction of a message fan-out bus. Producers call Push and
// consumers subscribe with Sub (pull channel) or Subscripter (push handler).
//
// Implementations:
//   - *ChanAdaptor: in-process fan-out over buffered channels (the default).
//   - *messaging.RedisAdaptor: backed by Redis Streams, only loaded when the
//     redis-backed messaging service is loaded (build tag !ram).
//
// The contract is intentionally loose: Push is best-effort async delivery.
// A chan adaptor may drop a message for a slow receiver (or wait up to a
// bounded timeout), while the redis adaptor persists messages in the stream so
// a slow consumer only delays its own consumer group.
type Adaptor[T any] interface {
	// Push publishes one message to all receivers.
	Push(data T)
	// Sub registers a named receiver and returns its pull channel. The
	// channel is closed when the adaptor stops. Returns nil if the receiver
	// name is already taken or the adaptor has already started.
	Sub(receiver string) <-chan T
	// Subscripter registers a named receiver driven by a handler goroutine.
	Subscripter(receiver string, fn Handler[T])
	// Start begins delivering messages to receivers. Receivers must be
	// registered before Start.
	Start()
	// Stop stops the adaptor and closes all receiver channels.
	Stop()
	// Receivers returns the registered receiver names.
	Receivers() []string
}

// Even Bus is good, but can't controll conconcurreny well and performance is not as good as chan,
// target to replace all eventbus with chanAdaptor.
type ChanAdaptor[T any] struct {
	sender       chan T
	receivers    map[string]chan T
	buf          int
	locker       sync.Mutex
	Started      bool
	DedupEnabled bool
	DedupWindow  time.Duration

	// BlockTimeout is the "third option" between dropping a message for a
	// slow receiver immediately and blocking everyone:
	//
	//   - BlockTimeout <= 0 (default): non-blocking send, drop for THIS
	//     receiver when its buffer is full. A slow receiver never blocks
	//     others, but its backlog is lost.
	//   - BlockTimeout > 0: give the slow receiver a bounded grace period —
	//     the message waits up to BlockTimeout for THIS receiver to drain.
	//     Short hiccups are absorbed (no loss); if the receiver is still full
	//     after the timeout the message is dropped for that receiver only, and
	//     subsequent messages are dropped immediately (fast path) until the
	//     receiver recovers, so one persistently stuck receiver cannot stall
	//     the whole fan-out.
	BlockTimeout time.Duration

	deduper   *Deduper
	dedupOnce sync.Once

	droppedTotal atomic.Uint64
	droppedBy    sync.Map // receiver -> *atomic.Uint64
	states       sync.Map // receiver -> *receiverState (slow-consumer bookkeeping)
}

func NewChanAdaptorWithDedupChecking[T any](buf int, dedupWindow time.Duration) *ChanAdaptor[T] {
	rr := NewChanAdaptor[T](buf)
	rr.DedupEnabled = true
	rr.DedupWindow = dedupWindow
	return rr
}

func NewChanAdaptor[T any](buf int) *ChanAdaptor[T] {
	if buf == 0 {
		buf = 1
	}
	rr := &ChanAdaptor[T]{
		sender:    make(chan T, buf),
		receivers: make(map[string]chan T),
		buf:       buf,
		locker:    sync.Mutex{},
	}
	OnServiceStarted(rr.Start)
	OnServiceStopping(func() {
		l := rr.getLogger()
		if rr == nil {
			l.Warn("chanAdaptor is nil, ignored.")
			return
		}
		close(rr.sender)
		l.Info("chanAdaptor stopped")
		time.Sleep(GraceShutdown)
	})
	return rr
}

func (ca *ChanAdaptor[T]) Push(data T) {
	if ca.dedupSeen(data) {
		ca.getLogger().Info("duplicate data skipped")
		return
	}
	ca.sender <- data
}

// TryPush attempts a non-blocking Push. It returns false when the sender
// buffer is full, so a caller (e.g. the redis consumer bridge) can keep the
// message pending for redelivery instead of blocking the consumer loop.
func (ca *ChanAdaptor[T]) TryPush(data T) bool {
	if ca.dedupSeen(data) {
		ca.getLogger().Info("duplicate data skipped")
		return true
	}
	select {
	case ca.sender <- data:
		return true
	default:
		ca.getLogger().Warn("chanAdaptor saturated, try push rejected")
		return false
	}
}

// dedupSeen reports whether the message is a duplicate and should be skipped.
// It also lazily initializes the deduper on first use.
func (ca *ChanAdaptor[T]) dedupSeen(data T) bool {
	if !ca.DedupEnabled || ca.DedupWindow <= 0 {
		return false
	}
	b, err := json.Marshal(data)
	if err != nil {
		ca.getLogger().Error("marshal data error", zap.Error(err))
		return false
	}
	ca.dedupOnce.Do(func() {
		ca.deduper = NewDeduper(ca.DedupWindow)
	})
	return ca.deduper.Seen(b)
}

func (ca *ChanAdaptor[T]) getLogger() *zap.Logger {
	var v T
	return zap.L().With(zap.String("adaptor", GetStructNameOnly(v)))
}

func (ca *ChanAdaptor[T]) Sub(receiver string) <-chan T {
	if receiver == "" {
		receiver = randstr.Hex(16)
	}
	l := ca.getLogger().With(zap.String("receiver", receiver))
	if ca.Started {
		l.Warn("adaptor is started, can't add new receiver")
		return nil
	}
	ca.locker.Lock()
	defer ca.locker.Unlock()
	if _, ok := ca.receivers[receiver]; ok {
		// return t
		l.Warn("receiver already exists")
		return nil
	}
	c := make(chan T, ca.buf)
	ca.receivers[receiver] = c
	l.Info("receiver suscribed")
	return c
}

func (ca *ChanAdaptor[T]) Subscripter(receiver string, fn Handler[T]) {
	l := ca.getLogger().With(zap.String("receiver", receiver))
	if fn == nil {
		l.Warn("handler is nil")
		return
	}
	c := ca.Sub(receiver)
	if c == nil {
		return
	}
	go func() {
		var wg sync.WaitGroup
		for v := range c {
			wg.Add(1)
			go func(v T) {
				defer wg.Done()
				if err := fn(v); err != nil {
					l.Error("handler error", zap.Error(err))
				}
			}(v)
		}
		wg.Wait()
	}()
}

// SubBatch registers a named receiver and returns its pull channel of batches
// ([]T). The batch is flushed when batchSize messages accumulated or
// flushInterval elapsed since the first message of the batch; the remaining
// partial batch is flushed on shutdown. batchSize / flushInterval <= 0 fall
// back to DefaultBatchSize / DefaultBatchInterval.
func (ca *ChanAdaptor[T]) SubBatch(receiver string, batchSize int, flushInterval time.Duration) <-chan []T {
	c := ca.Sub(receiver)
	if c == nil {
		return nil
	}
	out := make(chan []T, ca.buf)
	go ca.batchLoop(receiver, c, batchSize, flushInterval, func(batch []T) error {
		select {
		case out <- batch:
			return nil
		default:
			// pull consumer not keeping up: drop the batch and count it, so
			// the drop is observable via DroppedCount.
			ca.countDrop(receiver)
			return nil
		}
	}, func() { close(out) })
	return out
}

// SubscripterBatch registers a named receiver driven by a batch handler
// goroutine: messages are accumulated and fn is invoked once per batch instead
// of once per message (the old Subscripter spawns a goroutine per message,
// which is wasteful at monitor volume). A handler error is logged and — on the
// redis adaptor — keeps the whole batch pending for redelivery.
func (ca *ChanAdaptor[T]) SubscripterBatch(receiver string, batchSize int, flushInterval time.Duration, fn BatchHandler[T]) {
	l := ca.getLogger().With(zap.String("receiver", receiver))
	if fn == nil {
		l.Warn("handler is nil")
		return
	}
	c := ca.Sub(receiver)
	if c == nil {
		return
	}
	go ca.batchLoop(receiver, c, batchSize, flushInterval, func(batch []T) error {
		if err := fn(batch); err != nil {
			l.Error("batch handler error", zap.Int("len", len(batch)), zap.Error(err))
			return err
		}
		return nil
	}, nil)
}

// batchLoop accumulates messages from the receiver channel and flushes them as
// slices by size or interval. onBatch receives every flushed batch; onClose
// runs once when the underlying channel is closed (adaptor stopped) after the
// remaining partial batch is flushed.
func (ca *ChanAdaptor[T]) batchLoop(receiver string, c <-chan T, batchSize int, flushInterval time.Duration,
	onBatch func([]T) error, onClose func()) {
	defer func() {
		if onClose != nil {
			onClose()
		}
	}()
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = DefaultBatchInterval
	}
	l := ca.getLogger().With(
		zap.String("receiver", receiver),
		zap.Int("batchSize", batchSize),
		zap.Duration("interval", flushInterval),
	)

	pending := make([]T, 0, batchSize)
	var timer *time.Timer
	var timerC <-chan time.Time

	flush := func(reason string) {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make([]T, 0, batchSize)
		if timer != nil {
			timer.Stop()
			timer = nil
			timerC = nil
		}
		if err := onBatch(batch); err != nil {
			l.Error("batch flush failed", zap.String("reason", reason), zap.Int("len", len(batch)), zap.Error(err))
		}
	}

	for {
		select {
		case v, ok := <-c:
			if !ok {
				flush("closed")
				return
			}
			pending = append(pending, v)
			if len(pending) >= batchSize {
				flush("size")
			} else if timerC == nil {
				timer = time.NewTimer(flushInterval)
				timerC = timer.C
			}
		case <-timerC:
			flush("interval")
		}
	}
}

// make sure all receivers reg before start()
func (ca *ChanAdaptor[T]) Start() {
	l := ca.getLogger()
	if ca.Started {
		zap.L().Warn("chanAdaptor already started")
		return
	}
	l.Info("chanAdaptor started")
	ca.Started = true
	for v := range ca.sender {
		for receiver, c := range ca.receivers {
			ca.forward(receiver, c, v)
		}
	}

	for _, c := range ca.receivers {
		close(c)
	}
	l.Info("chanAdaptor and receivers were stopped.")
}

// forward delivers one message to one receiver.
//
// The "third option": instead of either (a) dropping instantly when the
// receiver buffer is full or (b) blocking the whole fan-out until the slow
// receiver drains, wait up to BlockTimeout for THIS receiver. When the timeout
// is exceeded the message is dropped for this receiver only and the receiver
// is marked as slow: while it stays slow, messages are dropped for it without
// waiting again, so a chronically stuck receiver degrades to per-receiver
// loss instead of stalling every other receiver and every producer.
func (ca *ChanAdaptor[T]) forward(receiver string, c chan T, v T) {
	l := ca.getLogger().With(zap.String("receiver", receiver))

	if ca.BlockTimeout > 0 {
		st := ca.receiverState(receiver)
		now := time.Now()
		if !st.lastDrop.IsZero() && now.Sub(st.lastDrop) < ca.BlockTimeout {
			// receiver was still full moments ago: don't wait again, drop for this receiver only.
			ca.countDrop(receiver)
			l.Warn("chanAdaptor receiver still slow, dropping message",
				zap.Duration("sinceLastDrop", now.Sub(st.lastDrop)))
			return
		}
		select {
		case c <- v:
			st.lastDrop = time.Time{}
			l.Debug("chanAdaptor fwd message", zap.String("receiver", receiver))
		case <-time.After(ca.BlockTimeout):
			// stamp the drop time AFTER the wait: for the next BlockTimeout the
			// receiver is treated as proven-slow and dropped without waiting.
			st.lastDrop = time.Now()
			ca.countDrop(receiver)
			l.Warn("chanAdaptor receiver channel full after timeout, dropping message", zap.String("receiver", receiver))
		}
		return
	}

	select {
	case c <- v:
		l.Debug("chanAdaptor fwd message", zap.String("receiver", receiver))
	default:
		ca.countDrop(receiver)
		l.Warn("chanAdaptor receiver channel full, dropping message", zap.String("receiver", receiver))
	}
}

// receiverState is the per-receiver slow-consumer bookkeeping used by the
// bounded-wait policy. It is only touched by the Start fan-out goroutine after
// the adaptor has started, so no extra locking is needed.
type receiverState struct {
	lastDrop time.Time
}

func (ca *ChanAdaptor[T]) receiverState(receiver string) *receiverState {
	raw, _ := ca.states.LoadOrStore(receiver, &receiverState{})
	return raw.(*receiverState)
}

func (ca *ChanAdaptor[T]) countDrop(receiver string) {
	ca.droppedTotal.Add(1)
	raw, _ := ca.droppedBy.LoadOrStore(receiver, &atomic.Uint64{})
	raw.(*atomic.Uint64).Add(1)
}

// TotalDropped returns how many messages were dropped since start due to full
// receiver buffers (bounded-wait timeout or immediate drop).
func (ca *ChanAdaptor[T]) TotalDropped() uint64 {
	return ca.droppedTotal.Load()
}

// DroppedCount returns how many messages were dropped for one receiver.
func (ca *ChanAdaptor[T]) DroppedCount(receiver string) uint64 {
	raw, ok := ca.droppedBy.Load(receiver)
	if !ok {
		return 0
	}
	return raw.(*atomic.Uint64).Load()
}

func (ca *ChanAdaptor[T]) Stop() {
	ca.getLogger().Info("chanAdaptor stopping")
	close(ca.sender)
	ca.Started = false
}

func (ca *ChanAdaptor[T]) Receivers() []string {
	return lo.Keys(ca.receivers)
}

type ErrorReport struct {
	AppName    string
	AppVersion string
	Uri        string
	FullStack  []byte
	Error      error
	HappendAT  time.Time
}

// ErrorAdaptor is the global error report adaptor for monitor error.
// Defaults to the in-process chan implementation; a process can swap it for a
// redis-streaming adaptor (see messaging.NewRedisAdaptor) before services
// start, e.g. the monitor-adaptor consumes redis streams directly.
var ErrorAdaptor Adaptor[ErrorReport] = NewChanAdaptor[ErrorReport](1000)

func (er ErrorReport) MarshalJSON() ([]byte, error) {
	type errorReportJSON struct {
		AppName    string    `json:"AppName"`
		AppVersion string    `json:"AppVersion"`
		Uri        string    `json:"Uri"`
		FullStack  []byte    `json:"FullStack"`
		Error      string    `json:"Error"`
		HappendAT  time.Time `json:"HappendAT"`
	}

	var errStr string
	if er.Error != nil {
		errStr = er.Error.Error()
	}

	return json.Marshal(errorReportJSON{
		AppName:    er.AppName,
		AppVersion: er.AppVersion,
		Uri:        er.Uri,
		FullStack:  er.FullStack,
		Error:      errStr,
		HappendAT:  er.HappendAT,
	})
}

func (er *ErrorReport) UnmarshalJSON(b []byte) error {
	type errorReportJSON struct {
		AppName    string          `json:"AppName"`
		AppVersion string          `json:"AppVersion"`
		Uri        string          `json:"Uri"`
		FullStack  []byte          `json:"FullStack"`
		Error      json.RawMessage `json:"Error"`
		HappendAT  time.Time       `json:"HappendAT"`
	}

	var v errorReportJSON
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	er.AppName = v.AppName
	er.AppVersion = v.AppVersion
	er.Uri = v.Uri
	er.FullStack = v.FullStack
	er.HappendAT = v.HappendAT
	er.Error = decodeErrorReportError(v.Error)
	return nil
}

func decodeErrorReportError(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return errors.New(s)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		if len(m) == 0 {
			return nil
		}
		if msg, ok := m["message"].(string); ok && msg != "" {
			return errors.New(msg)
		}
		if msg, ok := m["error"].(string); ok && msg != "" {
			return errors.New(msg)
		}
		return errors.New(string(raw))
	}

	var n any
	if err := json.Unmarshal(raw, &n); err == nil {
		if n == nil {
			return nil
		}
		return errors.New(string(raw))
	}

	return errors.New(string(raw))
}
