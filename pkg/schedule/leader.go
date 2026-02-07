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
	"go.uber.org/zap"
)

// LeaderElectionConfig defines configuration for leader election
type LeaderElectionConfig struct {
	Interval time.Duration `yaml:"interval"`
	TTL      time.Duration `yaml:"ttl"`
	Key      string        `yaml:"key"`
}

// LeaderElection manages leader election process
type LeaderElection struct {
	client   *redis.Client
	logger   *zap.Logger
	config   LeaderElectionConfig
	leaderID string
	isLeader int32 // 0=false, 1=true
	cancel   context.CancelFunc
}

// NewLeaderElection creates a new LeaderElection instance
func NewLeaderElection(client *redis.Client, logger *zap.Logger, cfg *LeaderElectionConfig) *LeaderElection {
	if cfg == nil {
		cfg = &LeaderElectionConfig{}
	}
	// Set defaults if not provided
	if cfg.Interval == 0 {
		cfg.Interval = 3 * time.Second
	}
	if cfg.TTL == 0 {
		cfg.TTL = 10 * time.Second
	}
	if cfg.Key == "" {
		cfg.Key = "scheduler:leader"
		if core.AppName != "" {
			safeAppName := strings.ReplaceAll(core.AppName, " ", "-")
			cfg.Key = fmt.Sprintf("scheduler:%s:leader", safeAppName)
		}
	}

	hostname, _ := os.Hostname()
	leaderID := fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())

	return &LeaderElection{
		client:   client,
		logger:   logger,
		config:   *cfg,
		leaderID: leaderID,
	}
}

// Start begins the leader election process in background
func (le *LeaderElection) Start() {
	if le.cancel != nil {
		return // Already started
	}
	ctx, cancel := context.WithCancel(context.Background())
	le.cancel = cancel

	le.logger.Info("starting leader election", zap.String("candidateID", le.leaderID), zap.Duration("interval", le.config.Interval), zap.Duration("ttl", le.config.TTL))

	go func() {
		ticker := time.NewTicker(le.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				le.elect(ctx)
			}
		}
	}()
}

// Stop stops the leader election process
func (le *LeaderElection) Stop() {
	if le.cancel != nil {
		le.cancel()
		le.cancel = nil
	}
}

func (le *LeaderElection) elect(ctx context.Context) {
	// 1. Try to become leader
	ok, err := le.client.SetNX(ctx, le.config.Key, le.leaderID, le.config.TTL).Result()
	if err != nil {
		le.logger.Error("leader election error", zap.Error(err))
		return
	}

	if ok {
		// Won election
		if atomic.CompareAndSwapInt32(&le.isLeader, 0, 1) {
			le.logger.Info("I am the leader now", zap.String("id", le.leaderID))
		}
		return
	}

	// 2. If not won, check if I am already the leader (renew lease)
	val, err := le.client.Get(ctx, le.config.Key).Result()
	if err != nil {
		le.logger.Error("leader check error", zap.Error(err))
		return
	}

	if val == le.leaderID {
		// Renew lease
		le.client.Expire(ctx, le.config.Key, le.config.TTL)
		if atomic.CompareAndSwapInt32(&le.isLeader, 0, 1) {
			le.logger.Info("I am the leader now (recovered)", zap.String("id", le.leaderID))
		}
	} else {
		// Someone else is leader
		if atomic.CompareAndSwapInt32(&le.isLeader, 1, 0) {
			le.logger.Info("I am no longer the leader", zap.String("new_leader", val))
		}
	}
}

// IsLeader returns true if the current instance is the leader
func (le *LeaderElection) IsLeader() bool {
	return atomic.LoadInt32(&le.isLeader) == 1
}

// GetLeaderID returns the candidate ID of this instance
func (le *LeaderElection) GetCandidateID() string {
	return le.leaderID
}
