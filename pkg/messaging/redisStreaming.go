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
	// DefaultPendingMinIdle 是批量消费重投（pending 检查）的最小空闲门槛兜底值。
	// 批量消费者把消息读进聚合窗口后，要等整批落库成功才统一 XAck，这段「在途」
	// 时间内消息本来就挂在 PEL 里；重投若碰到它们，就会与随后的正常落库重复写一次。
	DefaultPendingMinIdle = 60 * time.Second

	// MinBlockWait 是允许下发给 Redis 的最小 XREADGROUP BLOCK 时长。
	//
	// 为什么需要它：go-redis 把 Block 编码成 int64(block/time.Millisecond)，小于
	// 1ms 的正数会被整数除法截断成 0，而 Redis 的 BLOCK 0 语义是「永久阻塞」。与此
	// 同时客户端自己的读截止仍按原值 +10s 计算（baseClient.cmdTimeout）。两者叠加，
	// 「再等一会儿看有没有新消息」就变成「服务端无限期等待、客户端 10 秒后自己放弃」，
	// 日志里留下一条 `read tcp ...: i/o timeout`——纯客户端自造的超时，与 Redis 健康
	// 状况无关。亚毫秒的等待本来也聚合不到任何消息，任何调用方都不应下发这种值。
	MinBlockWait = time.Millisecond
)

// normalizeBlock 把「正数但小于 MinBlockWait」的阻塞时长抬到 MinBlockWait，其余值
// （0 = 不阻塞、负数 = 交给 go-redis 走 opt.ReadTimeout）原样返回，不做隐式改写。
// 详见 MinBlockWait 的说明。
func normalizeBlock(block time.Duration) time.Duration {
	if block > 0 && block < MinBlockWait {
		return MinBlockWait
	}
	return block
}

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
	// 发送端内存看门狗（缺省关闭，见 streamguard.go）：只守 messaging.guard.topics 白名单、
	// 进程内节流、多节点用 SET NX PX 抢锁只让一个节点真正裁剪；锁命令失败时 fail-open 照常裁。
	MaybeGuard(logger, msg.Client, topic)
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
//
// minIdle > 0 时跳过空闲时间不足 minIdle 的条目：批量消费者读出消息后要等整批
// 落库成功才 XAck，这些「在途」消息本身就在 PEL 里，重投它们会造成重复写入。
// 单条消费路径（ProcessPendings）传 0，行为不变。
func (msg *DefaultMessgingService) forEachPending(ctx context.Context, topic, group string, logger *zap.Logger,
	minIdle time.Duration, fn func(item redis.XMessage) error) {
	cmdPending, err := msg.Client.XPending(ctx, topic, group).Result()
	if err != nil {
		logger.Error("read pending message failed.", zap.Error(err))
		return
	}
	if cmdPending.Count == 0 {
		logger.Info("no pending messages")
		return
	}
	skippedTotal := 0
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
			// 在途消息（批量消费者已读出、正在聚合窗口里等整批落库）不重投：
			// 重投会插入一次，随后原批次落库再插一次 → 下游多一行重复数据。
			if minIdle > 0 && pendingItem.Idle < minIdle {
				skippedTotal++
				logger.Debug("skip in-flight pending message",
					zap.String("messageID", pendingItem.ID),
					zap.Duration("idle", pendingItem.Idle),
					zap.Duration("minIdle", minIdle))
				continue
			}
			// 按精确 ID 读回条目内容。不能用 XReadGroup 指定 ID：那是游标语义
			// （返回 ID 更大的条目），会把「下一条 pending」当当前条目处理，并让
			// 最后一条 pending 读不到内容、被下面误判成「已被裁剪」而 XACK 丢弃。
			readMsgs, err := msg.Client.XRange(ctx, topic, pendingItem.ID, pendingItem.ID).Result()
			if err != nil {
				logger.Error("read pending message failed.", zap.Error(err))
				return
			}
			if len(readMsgs) == 0 {
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
			for _, item := range readMsgs {
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

	if skippedTotal > 0 {
		logger.Info("pending messages still in flight, skipped",
			zap.Int("skipped", skippedTotal), zap.Duration("minIdle", minIdle))
	}

	logger.Info("process pending message done")
}

func (msg *DefaultMessgingService) ProcessPendings(ctx context.Context, topic, group string, processor Processor) {
	logger := msg.Logger.With(zap.String("topic", topic), zap.String("group", group))
	// 单条消费读完即 ack，没有聚合窗口，无需空闲门槛（0 = 不过滤）。
	msg.forEachPending(ctx, topic, group, logger, 0, func(item redis.XMessage) error {
		return msg.handleMessage(ctx, topic, group, logger, processor, item)
	})
}

// ProcessPendingBatch is the pending-redelivery counterpart for batch
// consumers: each pending message is replayed through the BatchProcessor one
// by one (a failed redelivery follows the same expiry/abandon rules as the
// single-message path).
func (msg *DefaultMessgingService) ProcessPendingBatch(ctx context.Context, topic, group string, processor BatchProcessor) {
	msg.ProcessPendingBatchWithMinIdle(ctx, topic, group, processor, 0)
}

// ProcessPendingBatchWithMinIdle 与 ProcessPendingBatch 相同，但跳过空闲时间小于
// minIdle 的 pending 条目：批量消费者读出的消息在整批落库成功前一直挂在 PEL 里，
// 这些在途消息不是失败消息，重投会与正常落库重复写一次。SubBatch 注册 pending
// 检查时按 flushInterval 计算门槛（见 pendingMinIdle）。
func (msg *DefaultMessgingService) ProcessPendingBatchWithMinIdle(ctx context.Context, topic, group string,
	processor BatchProcessor, minIdle time.Duration) {
	logger := msg.Logger.With(zap.String("topic", topic), zap.String("group", group))
	msg.forEachPending(ctx, topic, group, logger, minIdle, func(item redis.XMessage) error {
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
		Block:    normalizeBlock(block),
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
					// 剩余不足 MinBlockWait 就收工。不能拿亚毫秒的 remain 去阻塞：
					// go-redis 会把它截断成 BLOCK 0（服务端=永久阻塞），而客户端读
					// 截止只有 remain+10s，于是必然产生一条假的 i/o timeout（详见
					// MinBlockWait）。这点时间也聚合不到任何消息。
					if remain < MinBlockWait {
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

	// 批处理把消息读进聚合窗口后，要等整批落库成功才统一 XAck，这期间消息本就是
	// pending。pending 检查必须跳过这些在途消息，否则「重投一次 + 正常落库一次」
	// 就是下游的重复行（DuckDB monitor_* 表没有主键，重复只会越积越多）。
	minIdle := msg.pendingMinIdle(flushInterval)
	logger.Info("register pending check",
		zap.String("schedule", pschedule), zap.Duration("flushInterval", flushInterval), zap.Duration("minIdle", minIdle))

	schedule.CreateSchedule(fmt.Sprintf("check_pending_message/%s/%s", topic, group), pschedule, func() {
		msg.ProcessPendingBatchWithMinIdle(context.TODO(), topic, group, processor, minIdle)
	})

	return nil
}

// pendingMinIdle 计算批量消费重投的最小空闲门槛：聚合窗口最长 flushInterval，
// 之后还要等整批写库（quack 写入超时默认 30s），故取 2×flushInterval 且不低于
// DefaultPendingMinIdle；可用配置 messaging.pendingMinIdle 覆盖（>0 生效）。
func (msg *DefaultMessgingService) pendingMinIdle(flushInterval time.Duration) time.Duration {
	if v := viper.GetDuration("messaging.pendingMinIdle"); v > 0 {
		return v
	}
	minIdle := 2 * flushInterval
	if minIdle < DefaultPendingMinIdle {
		minIdle = DefaultPendingMinIdle
	}
	return minIdle
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
