//go:build !ram

package schedule

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

var (
	defaultLeaderElection *LeaderElection
)

func IsLeader() bool {
	if defaultLeaderElection == nil {
		return false
	}
	return defaultLeaderElection.IsLeader()
}

func CreateScheduledJob(jobname, schedule string, cmd func() error, opts ...ScheduleOptions) error {
	err := core.GetContainer().Invoke(func(logger *zap.Logger, redisClient *redis.Client) error {
		opt := &ScheduleOptions{}
		if len(opts) > 0 {
			opt = &opts[0]
		}
		// Consumer Logic: wrapped with Locker and History
		fn := wrapFuncJob(jobname, cmd, opt)

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
	config := &LeaderElectionConfig{}
	if err := viper.UnmarshalKey("schedule.leader", config); err != nil {
		logger.Warn("load schedule.leader config failed, use default", zap.Error(err))
	}

	defaultLeaderElection = NewLeaderElection(redisClient, logger, config)
	defaultLeaderElection.Start()
	return nil, nil
}

func init() {
	core.ProvideStartup(initScheduler)
}
