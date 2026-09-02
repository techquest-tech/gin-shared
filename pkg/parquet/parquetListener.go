package parquet

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

// DefaultTimeInterval is the default parquet time window: every hour.
const DefaultTimeInterval = time.Hour

// ParquetListener is the "default listener" for a core.Adaptor: it subscribes
// the adaptor and persists messages into parquet files partitioned by a
// configurable time window (default hourly). All messages of one window land
// in one file named by the window start time, so the file list itself is the
// time index.
//
// It works with any core.Adaptor implementation (chan or redis streaming), so
// the same code path serves both in-process and cross-process producers.
type ParquetListener[T any] struct {
	Adaptor  core.Adaptor[T]
	Receiver string
	// Setting holds the parquet storage settings (FsKey/Folder/Compress...).
	// When FilenamePattern is empty it defaults to "20060102/15" (hourly dirs).
	Setting *ParquetSetting
	// TimeInterval is the time window each parquet file covers. 0 means
	// viper "parquet.interval", then DefaultTimeInterval (1h).
	TimeInterval time.Duration
	// MaxRows optionally flushes a window early once the buffer reaches this
	// many rows. 0 means viper "parquet.maxRows", then unlimited.
	MaxRows int
	// BatchSize / BatchInterval are the adaptor-level batch delivery settings
	// (used only when the adaptor supports batched subscription). 0 falls back
	// to core.DefaultBatchSize / core.DefaultBatchInterval.
	BatchSize     int
	BatchInterval time.Duration

	writer        *ParquetDataService
	mu            sync.Mutex
	buf           []T
	winStart      time.Time
	interval      time.Duration
	maxRows       int
	batchSize     int
	batchInterval time.Duration
}

// NewParquetListener creates a listener for adaptor with default settings.
func NewParquetListener[T any](adaptor core.Adaptor[T]) *ParquetListener[T] {
	var v T
	return &ParquetListener[T]{
		Adaptor:  adaptor,
		Receiver: "parquet-" + strings.ToLower(core.GetStructNameOnly(v)),
		Setting:  DefaultListenerSetting(),
	}
}

// DefaultListenerSetting returns the parquet settings used by the default
// listener: local folder "data", hourly file name partition "20060102/15"
// (year/month/day/hour directories).
func DefaultListenerSetting() *ParquetSetting {
	return &ParquetSetting{
		Folder:          "data",
		FilenamePattern: "20060102/15",
		Compress:        "GZIP",
	}
}

func (pl *ParquetListener[T]) getLogger() *zap.Logger {
	var v T
	return zap.L().With(
		zap.String("service", "parquet-listener"),
		zap.String("receiver", pl.Receiver),
		zap.String("type", core.GetStructNameOnly(v)),
	)
}

func (pl *ParquetListener[T]) resolveInterval() time.Duration {
	if pl.TimeInterval > 0 {
		return pl.TimeInterval
	}
	if v := viper.GetDuration("parquet.interval"); v > 0 {
		return v
	}
	return DefaultTimeInterval
}

func (pl *ParquetListener[T]) resolveMaxRows() int {
	if pl.MaxRows > 0 {
		return pl.MaxRows
	}
	return viper.GetInt("parquet.maxRows")
}

func (pl *ParquetListener[T]) resolveBatchSize() int {
	if pl.BatchSize > 0 {
		return pl.BatchSize
	}
	return core.DefaultBatchSize
}

func (pl *ParquetListener[T]) resolveBatchInterval() time.Duration {
	if pl.BatchInterval > 0 {
		return pl.BatchInterval
	}
	return core.DefaultBatchInterval
}

// Start subscribes the adaptor and persists messages by time window until ctx
// is done (remaining messages are flushed on shutdown). It blocks until ctx
// is cancelled; use StartAsync to run it in the background.
func (pl *ParquetListener[T]) Start(ctx context.Context) error {
	logger := pl.getLogger()
	pl.interval = pl.resolveInterval()
	pl.maxRows = pl.resolveMaxRows()
	pl.batchSize = pl.resolveBatchSize()
	pl.batchInterval = pl.resolveBatchInterval()

	if pl.Setting.FilenamePattern == "" {
		pl.Setting.FilenamePattern = "20060102/15"
	}
	if pl.Adaptor == nil {
		logger.Error("adaptor is nil, parquet listener disabled")
		return nil
	}
	var v T
	pl.writer = &ParquetDataService{
		Setting: pl.Setting,
		Schema:  parquet.SchemaOf(v),
	}

	// Prefer batched delivery when the adaptor supports it: one handler call
	// per batch instead of a goroutine per message, and on the redis path the
	// batch is only acked after it is buffered here (window flush errors are
	// retried by the redis pending-check instead of silently lost).
	if b, ok := pl.Adaptor.(core.BatchingAdaptor[T]); ok {
		b.SubscripterBatch(pl.Receiver, pl.batchSize, pl.batchInterval, pl.onMessageBatch)
	} else {
		pl.Adaptor.Subscripter(pl.Receiver, pl.onMessage)
	}
	logger.Info("parquet listener started",
		zap.Duration("interval", pl.interval),
		zap.Int("maxRows", pl.maxRows),
		zap.String("filenamePattern", pl.Setting.FilenamePattern),
		zap.String("fsKey", pl.Setting.FsKey),
	)

	ticker := time.NewTicker(pl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("parquet listener stopping, flush remaining messages")
			pl.flush("shutdown")
			return nil
		case now := <-ticker.C:
			pl.flushExpiredWindows(now)
		}
	}
}

// StartAsync runs Start in a background goroutine.
func (pl *ParquetListener[T]) StartAsync(ctx context.Context) {
	go func() {
		if err := pl.Start(ctx); err != nil {
			pl.getLogger().Error("parquet listener exited with error", zap.Error(err))
		}
	}()
}

// onMessage buffers one message. Called concurrently by the adaptor.
func (pl *ParquetListener[T]) onMessage(v T) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.winStart.IsZero() {
		pl.winStart = time.Now().Truncate(pl.interval)
	}
	pl.buf = append(pl.buf, v)
	if pl.maxRows > 0 && len(pl.buf) >= pl.maxRows {
		pl.flushLocked("max_rows")
	}
	return nil
}

// onMessageBatch buffers a batch of messages (batched subscription path).
func (pl *ParquetListener[T]) onMessageBatch(vs []T) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if len(vs) == 0 {
		return nil
	}
	if pl.winStart.IsZero() {
		pl.winStart = time.Now().Truncate(pl.interval)
	}
	pl.buf = append(pl.buf, vs...)
	if pl.maxRows > 0 && len(pl.buf) >= pl.maxRows {
		pl.flushLocked("max_rows")
	}
	return nil
}

// flushExpiredWindows flushes every completed time window up to now, keeping
// one file per window.
func (pl *ParquetListener[T]) flushExpiredWindows(now time.Time) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for !pl.winStart.IsZero() && !now.Before(pl.winStart.Add(pl.interval)) {
		pl.flushLocked("time_window")
		pl.winStart = pl.winStart.Add(pl.interval)
	}
}

// flush flushes the current buffer unconditionally (shutdown).
func (pl *ParquetListener[T]) flush(reason string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.flushLocked(reason)
}

func (pl *ParquetListener[T]) flushLocked(reason string) {
	if len(pl.buf) == 0 {
		return
	}
	msgs := make([]any, len(pl.buf))
	for i, v := range pl.buf {
		msgs[i] = v
	}
	pl.buf = pl.buf[:0]
	win := pl.winStart
	if win.IsZero() {
		win = time.Now()
	}

	logger := pl.getLogger()
	logger.Info("parquet listener flush",
		zap.String("reason", reason),
		zap.Int("len", len(msgs)),
		zap.Time("window", win),
	)
	if pl.writer == nil {
		return
	}
	if _, err := pl.writer.WriteWindowed(msgs, win); err != nil {
		logger.Error("parquet listener flush failed",
			zap.String("reason", reason), zap.Error(err))
	}
}

// ListenAndPersist wires a startup that subscribes adaptor and persists its
// messages to parquet files by time window (default hourly). It is the
// one-call "default listener":
//
//	core.ProvideStartup(parquet.ListenAndPersist(monitor.TracingAdaptor))
//
// Optional viper config:
//
//	parquet:
//	  interval: 1h          # time window per file, default 1h
//	  maxRows: 100000       # optional max rows per file (0 = unlimited)
//	  fsKey: tracing.datapool  # storage config key (optional)
//	  folder: data          # storage folder (parquet setting)
//	  compress: GZIP        # default GZIP
func ListenAndPersist[T any](adaptor core.Adaptor[T]) func() core.Startup {
	return func() core.Startup {
		pl := NewParquetListener[T](adaptor)
		if sub := viper.Sub("parquet"); sub != nil {
			_ = sub.Unmarshal(pl.Setting)
		}
		if v := viper.GetString("parquet.fsKey"); v != "" {
			pl.Setting.FsKey = v
		}
		if v := viper.GetString("parquet.receiver"); v != "" {
			pl.Receiver = v
		}
		pl.StartAsync(core.RootCtx())
		return nil
	}
}
