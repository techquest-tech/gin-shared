//go:build !ram

package schedule

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"go.uber.org/zap"
)

var (
	leaderID string
	isLeader int32 // 0=false, 1=true
)

func IsLeader() bool {
	return atomic.LoadInt32(&isLeader) == 1
}

func CreateScheduledJob(jobname, schedule string, cmd func() error, opts ...ScheduleOptions) error {
	err := core.GetContainer().Invoke(func(logger *zap.Logger, locker locker.Locker, redisClient *redis.Client) error {
		opt := &ScheduleOptions{}
		if len(opts) > 0 {
			opt = &opts[0]
		}
		// Consumer Logic: wrapped with Locker and History
		fn := wrapFuncJob(jobname, locker, cmd, opt)

		// Producer Logic: Directly execute if Leader
		wrappedFn := func() {
			// Leader Check: Only the leader pod executes the job
			if !IsLeader() {
				return
			}
			fn()
		}

		// Pass wrappedFn as Producer (for Cron), fn as Consumer (for Jobs map - visual only now)
		return startCron(jobname, schedule, wrappedFn, fn, logger, opt)
	})
	if err != nil {
		zap.L().Error("schedule job failed.", zap.String("job", jobname), zap.Error(err))
	}
	return err
}

// // StartStreamWorker starts the distributed scheduler worker (Leader Election).
// // Kept name for compatibility, but now only starts leader election.
// // Deprecated: Use ginshared.Start() or ensure core.ProvideStartup(initScheduler) is called.
// func StartStreamWorker() {
// 	core.GetContainer().Invoke(func(redisClient *redis.Client, logger *zap.Logger) {
// 		_, _ = initScheduler(redisClient, logger)
// 	})
// }

func initScheduler(redisClient *redis.Client, logger *zap.Logger) (core.Startup, error) {
	// Start Leader Election
	startLeaderElection(redisClient, logger)
	return nil, nil
}

func startLeaderElection(client *redis.Client, logger *zap.Logger) {
	hostname, _ := os.Hostname()
	leaderID = fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	logger.Info("starting leader election", zap.String("candidateID", leaderID))

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		leaderKey := "scheduler:leader"
		if core.AppName != "" {
			safeAppName := strings.ReplaceAll(core.AppName, " ", "-")
			leaderKey = fmt.Sprintf("scheduler:%s:leader", safeAppName)
		}
		leaderTTL := 10 * time.Second

		for range ticker.C {
			ctx := context.Background()

			// 1. Try to become leader
			ok, err := client.SetNX(ctx, leaderKey, leaderID, leaderTTL).Result()
			if err != nil {
				logger.Error("leader election error", zap.Error(err))
				continue
			}

			if ok {
				// Won election
				if atomic.CompareAndSwapInt32(&isLeader, 0, 1) {
					logger.Info("I am the leader now", zap.String("id", leaderID))
				}
				continue
			}

			// 2. If not won, check if I am already the leader (renew lease)
			val, err := client.Get(ctx, leaderKey).Result()
			if err != nil {
				logger.Error("leader check error", zap.Error(err))
				continue
			}

			if val == leaderID {
				// Renew lease
				client.Expire(ctx, leaderKey, leaderTTL)
				if atomic.CompareAndSwapInt32(&isLeader, 0, 1) {
					logger.Info("I am the leader now (recovered)", zap.String("id", leaderID))
				}
			} else {
				// Someone else is leader
				if atomic.CompareAndSwapInt32(&isLeader, 1, 0) {
					logger.Info("I am no longer the leader", zap.String("new_leader", val))
				}
			}
		}
	}()
}

func init() {
	core.ProvideStartup(initScheduler)
}
