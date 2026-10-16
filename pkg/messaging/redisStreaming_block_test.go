//go:build !ram

package messaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// 本文件守护 2026-10-15 线上 monitor.schedule 反复报 i/o timeout 的根因：
// 聚合窗口循环把「正数但亚毫秒」的 remain 当 Block 下发给 go-redis，go-redis 用
// int64(block/time.Millisecond) 截断成 0，而 Redis 的 BLOCK 0 是「永久阻塞」；同时
// 客户端自己的读截止按 block+10s 计算（baseClient.cmdTimeout）。于是「再等一会儿
// 看有没有新消息」变成「服务端无限期等、客户端 10 秒后自己放弃」，日志里就是一条
// `read tcp ...: i/o timeout` —— 与 Redis 健康状况完全无关的假超时。

// TestNormalizeBlock 边界：只有「正数且小于 MinBlockWait」才被抬到 MinBlockWait，
// 0（不阻塞）与负数（交给 go-redis 走 opt.ReadTimeout）必须原样透传，不做隐式改写。
func TestNormalizeBlock(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{0, 0},
		{-1, -1},
		{-time.Second, -time.Second},
		{time.Nanosecond, MinBlockWait},
		{300 * time.Microsecond, MinBlockWait},
		{999 * time.Microsecond, MinBlockWait},
		{MinBlockWait, MinBlockWait},
		{1500 * time.Microsecond, 1500 * time.Microsecond},
		{30 * time.Second, 30 * time.Second},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, normalizeBlock(c.in), "normalizeBlock(%v)", c.in)
	}
}

// recordingConn 记录客户端实际写出的字节，用来断言 go-redis 到底把 block 编码成了
// 什么（实测为批量字符串 `block\r\n$1\r\n0\r\n`，即 block 值为 0）。
type recordingConn struct {
	net.Conn
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *recordingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.buf.Write(p)
	c.mu.Unlock()
	return c.Conn.Write(p)
}

func (c *recordingConn) snapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// xreadGroupWire 在空流上发起一次 XREADGROUP，返回客户端实际写出的命令行。
// 命令一旦写出就返回（不等阻塞结束），由本函数关掉 client 收尾。
func xreadGroupWire(t *testing.T, block time.Duration) string {
	t.Helper()

	rec := &recordingConn{}
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := net.Dial(network, addr)
			if err != nil {
				return nil, err
			}
			rec.Conn = conn
			return rec, nil
		},
	})
	pingCtx, cancelPing := context.WithTimeout(context.Background(), time.Second)
	defer cancelPing()
	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		t.Skipf("local redis not available, skip: %v", err)
	}

	ctx := context.Background()
	topic := fmt.Sprintf("test.block.wire.%d", time.Now().UnixNano())
	group := "block-test"
	assert.NoError(t, client.XGroupCreateMkStream(ctx, topic, group, "0").Err())

	done := make(chan struct{})
	go func() {
		defer close(done)
		// 空流：正常 BLOCK 会超时返回 redis.Nil，被截断成 0 则会一直挂住，
		// 两种情况都不影响下面只断言「写出去的字节」。
		_, _ = client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: "wire-test",
			Streams:  []string{topic, ">"},
			Count:    1,
			Block:    block,
		}).Result()
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rec.snapshot(), "xreadgroup") {
		time.Sleep(10 * time.Millisecond)
	}
	raw := rec.snapshot()

	client.Close() // 中断可能仍阻塞中的读
	<-done
	return raw
}

// TestSubMillisecondBlockIsSentAsBlockForever 是根因的线级证据：亚毫秒 Block 被
// 截断成 `block 0`（Redis 侧 = 永久阻塞）。这个断言如果哪天不成立，说明 go-redis
// 改了编码语义，MinBlockWait 的前提需要重新评估。
func TestSubMillisecondBlockIsSentAsBlockForever(t *testing.T) {
	raw := xreadGroupWire(t, 300*time.Microsecond)
	assert.Contains(t, raw, "block\r\n$1\r\n0\r\n",
		"亚毫秒 Block 必须被 go-redis 截断成 BLOCK 0（永久阻塞）；实际写出: %q", raw)
}

// TestNormalizeBlockFloorsWireBlock 修复效果：同样的亚毫秒入参，经 normalizeBlock
// 之后下发的必须是 `block 1`，服务端 1ms 就回 nil，客户端不会再自造 i/o timeout。
func TestNormalizeBlockFloorsWireBlock(t *testing.T) {
	raw := xreadGroupWire(t, normalizeBlock(300*time.Microsecond))
	assert.Contains(t, raw, "block\r\n$1\r\n1\r\n",
		"normalizeBlock 必须把亚毫秒抬到 1ms；实际写出: %q", raw)
	assert.NotContains(t, raw, "block\r\n$1\r\n0\r\n",
		"修复后绝不能再下发 BLOCK 0；实际写出: %q", raw)
}

// TestNormalizedBlockReturnsNilQuickly 行为验证：抬到 1ms 之后，空流上的阻塞读在
// 1ms 量级就以 redis.Nil 正常返回（不是错误、更不是 10 秒后的超时）。
func TestNormalizedBlockReturnsNilQuickly(t *testing.T) {
	client := testRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	topic := fmt.Sprintf("test.block.nil.%d", time.Now().UnixNano())
	group := "block-test"
	assert.NoError(t, client.XGroupCreateMkStream(ctx, topic, group, "0").Err())

	start := time.Now()
	_, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "nil-test",
		Streams:  []string{topic, ">"},
		Count:    1,
		Block:    normalizeBlock(300 * time.Microsecond),
	}).Result()
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, redis.Nil, "空流上的正常阻塞超时必须返回 redis.Nil")
	assert.Less(t, elapsed, 2*time.Second, "抬到 1ms 后必须立刻返回，不能等到客户端 10s 截止")
}

// TestSubMillisecondBlockTimesOutOnClient 端到端复现线上症状：亚毫秒 Block 下发后
// 服务端（按 BLOCK 0）永不返回，客户端在 block+10s 处自己放弃并报 i/o timeout。
// 需要约 10 秒，-short 下跳过。
func TestSubMillisecondBlockTimesOutOnClient(t *testing.T) {
	if testing.Short() {
		t.Skip("10 秒级复现，-short 下跳过")
	}
	client := testRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	topic := fmt.Sprintf("test.block.timeout.%d", time.Now().UnixNano())
	group := "block-test"
	assert.NoError(t, client.XGroupCreateMkStream(ctx, topic, group, "0").Err())

	start := time.Now()
	_, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "timeout-test",
		Streams:  []string{topic, ">"},
		Count:    1,
		Block:    300 * time.Microsecond, // 亚毫秒：线上被截断成 BLOCK 0
	}).Result()
	elapsed := time.Since(start)

	assert.Error(t, err, "亚毫秒 Block 必须复现出客户端自造的超时")
	assert.False(t, errors.Is(err, redis.Nil),
		"服务端从未返回 nil —— 它被要求永久阻塞，所以走的是客户端截止而非服务端超时")
	assert.Contains(t, err.Error(), "i/o timeout", "线上症状原文")
	assert.GreaterOrEqual(t, elapsed, 9*time.Second, "客户端在 block+10s 处放弃")
	assert.Less(t, elapsed, 12*time.Second)
}