package common

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useRedisMiniRedis 把 common.RDB 指向进程内 miniredis，并在测试结束还原。
func useRedisMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	previousClient := RDB
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, client.Ping(context.Background()).Err())
	RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		RDB = previousClient
	})
	return server
}

// TestRedisSetRejectsOversizedValue 生产回归（2026-09-29 全站 503 事故）：
// RedisSet 必须拒绝超过 MaxRedisValueBytes 的写入。生产曾把窗口内全量日志行
// （44 万行 / 290MB+）整体写入 maxmemory 仅 48MB 的 Redis，触发
// "OOM command not allowed"，主线程阻塞后限流与会话缓存全部失败，上游被
// Caddy 健康检查摘除，全站 503。拒写时 key 不得残留，普通值与边界值正常写入。
func TestRedisSetRejectsOversizedValue(t *testing.T) {
	server := useRedisMiniRedis(t)

	oversized := strings.Repeat("x", MaxRedisValueBytes+1)
	err := RedisSet("oversized:key", oversized, time.Minute)
	require.Error(t, err, "超限写入必须返回错误而不是静默成功")
	assert.False(t, server.Exists("oversized:key"), "被拒写的 key 不得残留在 Redis 中")

	require.NoError(t, RedisSet("normal:key", "small-value", time.Minute))
	stored, err := server.Get("normal:key")
	require.NoError(t, err)
	assert.Equal(t, "small-value", stored)

	// 边界值（恰好等于上限）仍然允许写入。
	atLimit := strings.Repeat("y", MaxRedisValueBytes)
	require.NoError(t, RedisSet("boundary:key", atLimit, time.Minute))
	assert.True(t, server.Exists("boundary:key"), "恰好等于上限的值应被接受")
}
