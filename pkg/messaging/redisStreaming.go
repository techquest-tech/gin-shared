//go:build !ram

package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	DefaultMsgLimit          = math.MaxInt16
	DefaultAttKey            = "payload"
	DefaultSchedule          = "@every 30m"
	DefaultDeadLetterDurtion = 8 * time.Hour //if messaging pending for more than this duration, will be put to dead letter
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
		err := processor(ctx, topic, group, []byte(vv))
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

func (msg *DefaultMessgingService) ProcessPendings(ctx context.Context, topic, group string, processor Processor) {
	logger := msg.Logger.With(zap.String("topic", topic))

	cmdPending, err := msg.Client.XPending(ctx, topic, group).Result()
	if err != nil {
		logger.Error("read pending message failed.", zap.Error(err))
		return
	}
	if cmdPending.Count > 0 {
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
					continue
				}
				for _, item := range readResult[0].Messages {
					err = msg.handleMessage(ctx, topic, group, logger, processor, item)
					if err != nil {
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
							// resp := msg.Client.XAdd(ctx, &redis.XAddArgs{
							// 	Stream: fmt.Sprintf("%s.%s.deadletter", topic, group),
							// 	Values: item.Values,
							// })
							// if resp.Err() != nil {
							// 	logger.Error("send to dead letter failed.", zap.Error(resp.Err()))
							// }
							payload := item.Values
							payload["ID"] = item.ID
							payload["consumer"] = c
							payload["topic"] = topic
							payload["duration"] = pendinged.String()

							AbandonedChan <- payload

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
	} else {
		logger.Info("no pending messages")
	}
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

func init() {
	core.Provide(func(client *redis.Client, logger *zap.Logger) (MessagingService, *DefaultMessgingService) {
		d := &DefaultMessgingService{
			Client: client,
			Logger: logger,
		}
		sub := viper.Sub("messaging")
		if sub != nil {
			logger.Info("get settings.", zap.Any("keys", sub.AllKeys()))
			sub.Unmarshal(d)
			startIndex := len("settings.")
			for _, key := range sub.AllKeys() {
				logger.Info("get setting.", zap.String("key", key), zap.Any("value", sub.Get(key)))
				k := key[startIndex:]
				value := sub.GetInt64(key)
				d.Settings[k] = value
			}
		}
		return d, d
	})
}
