//go:build !ram

package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

func CreateScheduledJob(jobname, schedule string, cmd func() error, opts ...ScheduleOptions) error {
	err := core.GetContainer().Invoke(func(logger *zap.Logger, locker locker.Locker, redisClient *redis.Client) error {
		opt := &ScheduleOptions{}
		if len(opts) > 0 {
			opt = &opts[0]
		}
		// Consumer Logic: wrapped with Locker and History
		fn := wrapFuncJob(jobname, locker, cmd, opt)

		// Producer Logic: Stream Producer
		wrappedFn := func() {
			// Determine deduplication precision based on schedule
			// Default to Minute precision for standard cron
			timeFormat := "2006-01-02-15-04"

			// For @every schedule with duration < 1 minute, use Second precision
			if strings.HasPrefix(schedule, "@every ") {
				if d, err := time.ParseDuration(strings.TrimPrefix(schedule, "@every ")); err == nil && d < time.Minute {
					timeFormat = "2006-01-02-15-04-05"
				}
			}

			// Use a simple deduplication key to prevent all pods from flooding the stream
			// Key format: job:trigger:{jobname}:{timestamp}
			now := time.Now()
			dedupKey := fmt.Sprintf("job:trigger:%s:%s", jobname, now.Format(timeFormat))

			// Try to set NX with short TTL
			ok, err := redisClient.SetNX(context.Background(), dedupKey, "1", 2*time.Minute).Result()
			if err != nil {
				logger.Error("failed to set dedup key", zap.Error(err))
				return
			}
			if !ok {
				// Already triggered by another pod
				logger.Debug("job trigger skipped (dedup)", zap.String("job", jobname))
				return
			}

			// Produce to stream
			_, err = redisClient.XAdd(context.Background(), &redis.XAddArgs{
				Stream: StreamKey,
				Values: map[string]interface{}{
					"job": jobname,
					"ts":  now.Unix(),
				},
				// Optional: Trim stream to keep size manageable
				MaxLen: 1000,
			}).Result()

			if err != nil {
				logger.Error("failed to push job to stream", zap.String("job", jobname), zap.Error(err))
			} else {
				logger.Info("pushed job to stream", zap.String("job", jobname))
			}
		}

		// Pass wrappedFn as Producer (for Cron), fn as Consumer (for Jobs map)
		return startCron(jobname, schedule, wrappedFn, fn, logger, opt)
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
}

func StartStreamWorker() {
	core.GetContainer().Invoke(func(redisClient *redis.Client, logger *zap.Logger) {
		_, _ = initStreamWorker(redisClient, logger)
	})
}

func initStreamWorker(redisClient *redis.Client, logger *zap.Logger) (core.Startup, error) {
	// Create consumer group if not exists
	err := redisClient.XGroupCreateMkStream(context.Background(), StreamKey, GroupKey, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		logger.Error("create consumer group failed", zap.Error(err))
		return nil, nil
	}

	// Start worker loop
	go func() {
		consumerName := fmt.Sprintf("%s-%s", ConsumerKey, core.AppName)
		for {
			// Read new messages
			entries, err := redisClient.XReadGroup(context.Background(), &redis.XReadGroupArgs{
				Group:    GroupKey,
				Consumer: consumerName,
				Streams:  []string{StreamKey, ">"},
				Count:    1,
				Block:    5 * time.Second,
			}).Result()

			if err != nil {
				if err != redis.Nil {
					logger.Error("read stream failed", zap.Error(err))
				}
				continue
			}

			for _, stream := range entries {
				for _, msg := range stream.Messages {
					processStreamMsg(msg, redisClient, logger)
				}
			}
		}
	}()

	// Start monitor/recovery loop (for pending messages)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		consumerName := fmt.Sprintf("%s-%s", ConsumerKey, core.AppName)
		for range ticker.C {
			// Check pending messages
			pending, err := redisClient.XPendingExt(context.Background(), &redis.XPendingExtArgs{
				Stream: StreamKey,
				Group:  GroupKey,
				Start:  "-",
				End:    "+",
				Count:  10,
				// Messages pending for more than 1 minute
				Idle: 1 * time.Minute,
			}).Result()

			if err != nil {
				logger.Error("check pending failed", zap.Error(err))
				continue
			}

			for _, p := range pending {
				// Claim message
				msgs, err := redisClient.XClaim(context.Background(), &redis.XClaimArgs{
					Stream:   StreamKey,
					Group:    GroupKey,
					Consumer: consumerName,
					MinIdle:  1 * time.Minute,
					Messages: []string{p.ID},
				}).Result()

				if err != nil {
					logger.Error("claim failed", zap.Error(err))
					continue
				}

				for _, msg := range msgs {
					logger.Info("claimed pending message", zap.String("msgID", msg.ID))
					processStreamMsg(msg, redisClient, logger)
				}
			}
		}
	}()

	return nil, nil
}

func init() {
	core.ProvideStartup(initStreamWorker)
}

func processStreamMsg(msg redis.XMessage, client *redis.Client, logger *zap.Logger) {
	jobName, ok := msg.Values["job"].(string)
	if !ok {
		logger.Error("invalid job message", zap.Any("msg", msg))
		// Ack invalid message to remove it
		client.XAck(context.Background(), StreamKey, GroupKey, msg.ID)
		return
	}

	logger.Info("received job from stream", zap.String("job", jobName), zap.String("msgID", msg.ID))

	fn, exists := jobs[jobName]
	if exists {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("job panic", zap.Any("err", r))
				}
			}()
			fn()
		}()

		client.XAck(context.Background(), StreamKey, GroupKey, msg.ID)
		logger.Info("job finished and acked", zap.String("job", jobName))
	} else {
		logger.Warn("job not found locally", zap.String("job", jobName))
		// Ack unknown job to prevent infinite redelivery
		client.XAck(context.Background(), StreamKey, GroupKey, msg.ID)
	}
}
