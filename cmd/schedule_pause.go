//go:build !ram

package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func schedulerLeaderKey() string {
	if key := strings.TrimSpace(viper.GetString("schedule.leader.key")); key != "" {
		return key
	}
	key := "scheduler:leader"
	if core.AppName != "" {
		safeAppName := strings.ReplaceAll(core.AppName, " ", "-")
		key = fmt.Sprintf("scheduler:%s:leader", safeAppName)
	}
	return key
}

var SchedulePauseCmd = &cobra.Command{
	Use:   "schedule-pause [minutes]",
	Short: "pause schedule job execution by taking over leader election key",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		minutes, _ := cmd.Flags().GetInt("minutes")
		if len(args) == 1 {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid minutes: %w", err)
			}
			minutes = n
		}
		if minutes <= 0 {
			return fmt.Errorf("minutes must be > 0")
		}

		return core.GetContainer().Invoke(func(logger *zap.Logger, redisClient *redis.Client) error {
			key := schedulerLeaderKey()

			hostname, _ := os.Hostname()
			val := fmt.Sprintf("paused-by-cli:%s:%d", hostname, time.Now().UnixNano())
			dur := time.Duration(minutes) * time.Minute

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := redisClient.Set(ctx, key, val, dur).Err(); err != nil {
				logger.Error("pause schedule failed", zap.String("key", key), zap.Error(err))
				return err
			}

			ttl, err := redisClient.TTL(ctx, key).Result()
			if err != nil {
				logger.Warn("get key ttl failed", zap.String("key", key), zap.Error(err))
			}

			fmt.Printf("schedule paused: key=%s ttl=%s\n", key, ttl)
			return nil
		})
	},
}

var ScheduleResumeCmd = &cobra.Command{
	Use:   "schedule-resume",
	Short: "resume schedule job execution by releasing leader election key",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return core.GetContainer().Invoke(func(logger *zap.Logger, redisClient *redis.Client) error {
			key := schedulerLeaderKey()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			removed, err := redisClient.Del(ctx, key).Result()
			if err != nil {
				logger.Error("resume schedule failed", zap.String("key", key), zap.Error(err))
				return err
			}

			fmt.Printf("schedule resumed: key=%s removed=%d\n", key, removed)
			return nil
		})
	},
}

func init() {
	SchedulePauseCmd.Flags().IntP("minutes", "m", 10, "pause duration in minutes")
}

