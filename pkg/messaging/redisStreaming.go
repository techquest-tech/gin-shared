//go:build !ram

package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/schedule"
	"go.uber.org/zap"
)

// var AbandonedChan chan any
var ConsumerName string
var ResetTopics []string

const (
	DefaultMsgLimit          = 5000 //math.MaxInt16, redis loading too much, make it 5K
	DefaultAttKey            = "payload"
	DefaultSchedule          = "@every 30m"
	DefaultDeadLetterDurtion = 72 * time.Hour //if messaging pending for more than this duration, will be put to dead letter
)

type MessagnePending struct {
	Topic    string
	Group    string
	Schedule string
	MaxBatch int
	Limit    int
}

type DefaultMessgingService struct {
	Logger          *zap.Logger
	Client          *redis.Client
	PendingSchedule string
	Settings        map[string]int64 // settings for streaming limit settings. default 10000
}

func (msg *DefaultMessgingService) Pub(ctx context.Context, topic string, payload any) error {
	logger := msg.Logger.With(zap.String("topic", topic))
	logger.Debug("start to pub message")

	limit := int64(DefaultMsgLimit)
	if v, ok := msg.Settings[topic]; ok {
		limit = v
		logger.Debug("set the topic limit", zap.Int64("limit", limit))
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp := msg.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		Values: map[string]string{DefaultAttKey: string(raw)}, // TsKey: time.Now().Format(time.RFC3339),
		MaxLen: limit,
	})
	if resp.Err() != nil {
		logger.Error("pub message failed.", zap.Error(resp.Err()), zap.Any("payload", payload))
		return resp.Err()
	}
	logger.Debug("pub message done")
	return nil
}

func (msg *DefaultMessgingService) handleMessage(ctx context.Context, topic, group string, logger *zap.Logger,
	processor Processor, v redis.XMessage) error {
	id := v.ID
	value := v.Values
	if value == nil {
		logger.Warn("message value is empty", zap.String("messageID", id))
	} else {
		logger.Debug("recieved message", zap.String("ID", id), zap.Any("value", value))
		raw := value[DefaultAttKey]
		vv := raw.(string)
		err := processor(WithMessageID(ctx, id), topic, group, []byte(vv))
		if err != nil {
			logger.Error("processor return error", zap.Error(err))
			return err
		}
	}

	resp := msg.Client.XAck(ctx, topic, group, id)
	if resp.Err() != nil {
		logger.Error("ack message failed.", zap.Error(resp.Err()))
	}
	logger.Debug("process done")
	return nil
}

// handleBatchMessage handles one message through a BatchProcessor (pending
// redelivery feeds messages one by one even for batch consumers). The message
// is acked only after the processor returns nil, otherwise it stays pending.
func (msg *DefaultMessgingService) handleBatchMessage(ctx context.Context, topic, group string, logger *zap.Logger,
	processor BatchProcessor, v redis.XMessage) error {
	id := v.ID
	value := v.Values
	if value == nil {
		logger.Warn("message value is empty", zap.String("messageID", id))
		return nil
	}
	raw, ok := value[DefaultAttKey].(string)
	if !ok {
		logger.Warn("message value has no string payload", zap.String("messageID", id))
		return nil
	}
	if err := processor(WithMessageID(ctx, id), topic, group, [][]byte{[]byte(raw)}); err != nil {
		logger.Error("batch processor return error", zap.Error(err))
		return err
	}
	resp := msg.Client.XAck(ctx, topic, group, id)
	if resp.Err() != nil {
		logger.Error("ack message failed.", zap.Error(resp.Err()))
	}
	logger.Debug("process done")
	return nil
}

// forEachPending iterates every pending message of a consumer group and calls
// fn for each. When fn fails, the message's pending age is checked: messages
// pending longer than DefaultDeadLetterDurtion (or whose ID cannot be parsed)
// are acknowledged and pushed to AbandonedChan, the others stay pending for a
// later round.
func (msg *DefaultMessgingService) forEachPending(ctx context.Context, topic, group string, logger *zap.Logger,
	fn func(item redis.XMessage) error) {
	cmdPending, err := msg.Client.XPending(ctx, topic, group).Result()
	if err != nil {
		logger.Error("read pending message failed.", zap.Error(err))
		return
	}
	if cmdPending.Count == 0 {
		logger.Info("no pending messages")
		return
	}
	for c, p := range cmdPending.Consumers {
		xrangeResult, err := msg.Client.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream:   topic,
			Group:    group,
			Count:    p,
			Start:    cmdPending.Lower,
			End:      cmdPending.Higher,
			Consumer: c,
		}).Result()
		if err != nil {
			logger.Warn("read pending message by XPendingExt failed. will try next round.", zap.String("topic", topic), zap.Error(err))
			continue
		}
		logger.Info("read pending message done.", zap.Int("count", len(xrangeResult)))
		for _, pendingItem := range xrangeResult {
			readResult, err := msg.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: pendingItem.Consumer,
				Streams:  []string{topic, pendingItem.ID},
				Count:    1,
				Block:    0,
			}).Result()
			if err != nil {
				logger.Error("read pending message failed.", zap.Error(err))
				return
			}
			if len(readResult) == 0 {
				// 条目仍在 PEL 但按精确 ID 读不到内容 = 已被流裁剪（MAXLEN/XTRIM 只从
				// 流头删，不看消费组）。数据在裁剪时已物理删除，重投不可能成功：直接
				// XACK 移除墓碑，避免 PEL 永久卡死（pending 只增不减）。72h dead-letter
				// 只挂在处理器报错分支（见下方 fn err 处理），覆盖不到本场景，故在此补齐。
				// XACK 对不存在的条目是幂等 no-op，重复/并发清理无副作用。
				logger.Warn("pending entry trimmed from stream, discard it",
					zap.String("messageID", pendingItem.ID),
					zap.String("topic", topic), zap.String("group", group))
				if resp := msg.Client.XAck(ctx, topic, group, pendingItem.ID); resp.Err() != nil {
					logger.Error("ack trimmed pending message failed.", zap.Error(resp.Err()))
				}
				continue
			}
			for _, item := range readResult[0].Messages {
				if err := fn(item); err != nil {
					logger.Error("process pending message failed. ", zap.Error(err))
					strT := item.ID
					index := strings.IndexRune(item.ID, '-')
					if index > 0 {
						strT = item.ID[:index]
					}

					unixTimeint, err := strconv.ParseInt(strT, 10, 64)
					if err != nil {
						logger.Warn("convert pending message id failed.", zap.Error(err))
					}
					pendinged := time.Since(time.Unix(unixTimeint/1000, 0))
					logger.Info("checking pending duration", zap.Duration("pendinged", pendinged))
					if pendinged >= DefaultDeadLetterDurtion || err != nil {
						logger.Warn("pending message expired, abandoned.", zap.String("duration", pendinged.String()))
						payload := item.Values
						payload["ID"] = item.ID
						payload["consumer"] = c
						payload["topic"] = topic
						payload["duration"] = pendinged.String()

						// AbandonedChan is a global sink that may have no
						// reader (nothing in the workspace consumes it), so
						// never block on it — otherwise a single abandoned
						// message would hang the whole pending check.
						select {
						case AbandonedChan <- payload:
						default:
							logger.Warn("abandoned chan full or unread, dropped", zap.String("messageID", item.ID))
						}

						ackResp := msg.Client.XAck(ctx, topic, group, item.ID)
						if ackResp.Err() != nil {
							logger.Error("ack pending message failed.", zap.Error(ackResp.Err()))
						}
					}
				}
			}
		}
	}

	logger.Info("process pending message done")
}

func (msg *DefaultMessgingService) ProcessPendings(ctx context.Context, topic, group string, processor Processor) {
	logger := msg.Logger.With(zap.String("topic", topic), zap.String("group", group))
	msg.forEachPending(ctx, topic, group, logger, func(item redis.XMessage) error {
		return msg.handleMessage(ctx, topic, group, logger, processor, item)
	})
}

// ProcessPendingBatch is the pending-redelivery counterpart for batch
// consumers: each pending message is replayed through the BatchProcessor one
// by one (a failed redelivery follows the same expiry/abandon rules as the
// single-message path).
func (msg *DefaultMessgingService) ProcessPendingBatch(ctx context.Context, topic, group string, processor BatchProcessor) {
	logger := msg.Logger.With(zap.String("topic", topic), zap.String("group", group))
	msg.forEachPending(ctx, topic, group, logger, func(item redis.XMessage) error {
		return msg.handleBatchMessage(ctx, topic, group, logger, processor, item)
	})
}

func (msg *DefaultMessgingService) checkAndCreate(ctx context.Context, topic, group string) error {
	logger := msg.Logger.With(zap.String("topic", topic))
	// check if topic exists, if not create one
	cmd := msg.Client.XInfoStream(ctx, topic)
	if cmd.Err() != nil {
		logger.Info("topic not exists, create one.", zap.Error(cmd.Err()))
		resp := msg.Client.XAdd(ctx, &redis.XAddArgs{
			Stream: topic,
			Values: map[string]any{DefaultAttKey: ""},
		})
		if resp.Err() != nil {
			logger.Error("create topic failed.", zap.Error(resp.Err()))
			return resp.Err()
		}
		// msg.Client.XGroupSetID(ctx, topic, group, "0")
		logger.Info("topic created")
	}
	err := msg.Client.XGroupCreate(ctx, topic, group, "$").Err()
	if err != nil {
		logger.Warn("group might be created.", zap.Error(err), zap.String("group", group))
	}
	return nil
}

func (msg *DefaultMessgingService) Sub(ctx context.Context, topic, group string, processor Processor) error {
	if processor == nil {
		return errors.New("processor is empty")
	}

	logger := msg.Logger.With(zap.String("topic", topic))
	err := msg.checkAndCreate(ctx, topic, group)
	if err != nil {
		return err
	}

	if lo.Contains(ResetTopics, topic) {
		msg.Client.XGroupSetID(ctx, topic, group, "0")
		logger.Info("reset topic", zap.String("topic", topic))
	}

	go func() {
		if ConsumerName == "" {
			hostname, err := os.Hostname()
			if err != nil {
				logger.Error("failed to get hostname, just make it empty", zap.Error(err))
			}

			ConsumerName = hostname //+ "-" + time.Now().Format("20060102150405")
		}

		logger.Info("start consumer", zap.String("group", group),
			zap.String("topic", topic), zap.String("consumer", ConsumerName))

		for {
			cmd := msg.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: ConsumerName,
				Streams:  []string{topic, ">"},
				Count:    0,
			})

			vv, err := cmd.Result()
			if err != nil {
				// XReadGroup Block 超时无消息时 go-redis 返回 redis.Nil：属正常空闲，
				// 不是错误，直接进入下一轮（避免每秒刷 error 日志 + 多余 checkAndCreate）。
				if errors.Is(err, redis.Nil) {
					continue
				}
				logger.Error("received message failed.", zap.Error(err))
				//just in case someone else delete the topic and crash the receiver
				go msg.checkAndCreate(ctx, topic, group)

				time.Sleep(time.Second)
				continue
			}
			for _, v := range vv[0].Messages {
				msg.handleMessage(ctx, topic, group, logger, processor, v)
			}
		}
	}()

	pschedule := msg.PendingSchedule
	if pschedule == "" {
		pschedule = DefaultSchedule
	}

	schedule.CreateSchedule(fmt.Sprintf("check_pending_message/%s/%s", topic, group), pschedule, func() {
		msg.ProcessPendings(context.TODO(), topic, group, processor)
	})

	return nil
}

// readGroupOnce 执行一次 XREADGROUP 批量读取。
// 返回 (nil, nil) 表示阻塞超时无消息（redis.Nil，属正常空闲）；返回 (nil, err)
// 表示真实错误；否则返回本次读到的消息。
func (msg *DefaultMessgingService) readGroupOnce(ctx context.Context, topic, group string, batchSize int, block time.Duration) ([]redis.XMessage, error) {
	vv, err := msg.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: ConsumerName,
		Streams:  []string{topic, ">"},
		Count:    int64(batchSize),
		Block:    block,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	if len(vv) == 0 || len(vv[0].Messages) == 0 {
		return nil, nil
	}
	return vv[0].Messages, nil
}

// SubBatch registers a batch consumer on the stream. The consumer reads up to
// batchSize messages (waiting up to flushInterval for more) and hands the whole
// group to processor in ONE call; only after processor returns nil are all the
// messages acknowledged together. On error the whole batch stays pending and is
// redelivered by the pending-check schedule — a downstream write failure no
// longer silently loses data that was already read out of the stream.
func (msg *DefaultMessgingService) SubBatch(ctx context.Context, topic, group string,
	batchSize int, flushInterval time.Duration, processor BatchProcessor) error {
	if processor == nil {
		return errors.New("processor is empty")
	}

	logger := msg.Logger.With(zap.String("topic", topic))
	err := msg.checkAndCreate(ctx, topic, group)
	if err != nil {
		return err
	}

	if lo.Contains(ResetTopics, topic) {
		msg.Client.XGroupSetID(ctx, topic, group, "0")
		logger.Info("reset topic", zap.String("topic", topic))
	}

	if batchSize <= 0 {
		batchSize = 0 // 0 = all available in one read
	}
	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	go func() {
		if ConsumerName == "" {
			hostname, err := os.Hostname()
			if err != nil {
				logger.Error("failed to get hostname, just make it empty", zap.Error(err))
			}

			ConsumerName = hostname //+ "-" + time.Now().Format("20060102150405")
		}

		logger.Info("start batch consumer", zap.String("group", group),
			zap.String("topic", topic), zap.String("consumer", ConsumerName),
			zap.Int("batchSize", batchSize), zap.Duration("flushInterval", flushInterval))

		for {
			// 首读：Block=flushInterval 等第一条消息。空闲超时（redis.Nil）属正常，
			// 直接下一轮，避免空转刷日志。
			msgs, err := msg.readGroupOnce(ctx, topic, group, batchSize, flushInterval)
			if err != nil {
				logger.Error("received message failed.", zap.Error(err))
				//just in case someone else delete the topic and crash the receiver
				go msg.checkAndCreate(ctx, topic, group)

				time.Sleep(time.Second)
				continue
			}
			if len(msgs) == 0 {
				continue
			}

			// 聚合窗口：距首读至多 flushInterval 内继续取新消息，凑满 batchSize 或窗口
			// 耗尽才整批交给 processor。低流量下把若干条小消息合成一条大批写，显著降低
			// 下游按“每条消息一次 INSERT/请求”处理的开销（如 MyDuck 单写者 + temp table
			// 场景：同一窗口的 N 行合成一条多行 INSERT）。语义安全性不变：已读出未确认的
			// 消息在 XREADGROUP 投递时即进入 pending，聚合窗口内崩溃由 pending-check 重投；
			// ack 仍只在 processor 成功后统一发出，不引入丢数据窗口。batchSize<=0（一次取
			// 全部可用）时不聚合，保持原有即时处理语义。
			if batchSize > 0 {
				deadline := time.Now().Add(flushInterval)
				for len(msgs) < batchSize {
					remain := time.Until(deadline)
					if remain <= 0 {
						break
					}
					more, err2 := msg.readGroupOnce(ctx, topic, group, batchSize-len(msgs), remain)
					if err2 != nil {
						// 聚合途中 Redis 抖动：不丢已读消息，按已聚合的整批处理，下次再续。
						logger.Warn("accumulate read failed, flush what we have", zap.Error(err2))
						break
					}
					if len(more) == 0 {
						break // 窗口耗尽且无更多消息
					}
					msgs = append(msgs, more...)
				}
			}

			ids := make([]string, 0, len(msgs))
			payloads := make([][]byte, 0, len(msgs))
			for _, v := range msgs {
				ids = append(ids, v.ID)
				if raw, ok := v.Values[DefaultAttKey].(string); ok {
					payloads = append(payloads, []byte(raw))
				}
			}
			if len(payloads) == 0 {
				// nothing parseable; ack the ids so they don't stay pending forever
				ackResp := msg.Client.XAck(ctx, topic, group, ids...)
				if ackResp.Err() != nil {
					logger.Error("batch ack failed.", zap.Error(ackResp.Err()))
				}
				continue
			}

			if err := processor(WithMessageID(ctx, msgs[0].ID), topic, group, payloads); err != nil {
				// keep the whole batch pending: the pending-check schedule
				// redelivers it later (or abandons it after the dead-letter
				// duration) instead of silently losing it.
				logger.Error("batch processor error, keep messages pending",
					zap.Int("count", len(payloads)), zap.Error(err))
				time.Sleep(time.Second)
				continue
			}

			resp := msg.Client.XAck(ctx, topic, group, ids...)
			if resp.Err() != nil {
				logger.Error("batch ack failed.", zap.Error(resp.Err()))
			}
			logger.Debug("batch process done", zap.Int("count", len(payloads)))
		}
	}()

	pschedule := msg.PendingSchedule
	if pschedule == "" {
		pschedule = DefaultSchedule
	}

	schedule.CreateSchedule(fmt.Sprintf("check_pending_message/%s/%s", topic, group), pschedule, func() {
		msg.ProcessPendingBatch(context.TODO(), topic, group, processor)
	})

	return nil
}

func init() {
	core.Provide(func(client *redis.Client, logger *zap.Logger) (MessagingService, *DefaultMessgingService) {
		d := &DefaultMessgingService{
			Client:   client,
			Logger:   logger,
			Settings: map[string]int64{},
		}
		sub := viper.Sub("messaging")
		if sub != nil {
			logger.Info("get settings.", zap.Any("keys", sub.AllKeys()))
			sub.Unmarshal(d)
			for _, key := range sub.AllKeys() {
				logger.Info("get setting.", zap.String("key", key), zap.Any("value", sub.Get(key)))
				if !strings.HasPrefix(key, "settings.") {
					continue
				}
				k := strings.TrimPrefix(key, "settings.")
				d.Settings[k] = sub.GetInt64(key)
			}
		}
		return d, d
	})
}
