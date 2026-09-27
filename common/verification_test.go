package common

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerificationMemoryPath 覆盖验证码内存存储语义：注册/校验/一次性消费/删除/过期。
func TestVerificationMemoryPath(t *testing.T) {
	prevEnabled, prevRDB := RedisEnabled, RDB
	RedisEnabled, RDB = false, nil
	defer func() { RedisEnabled, RDB = prevEnabled, prevRDB }()

	key := "memory-path@example.com"
	RegisterVerificationCodeWithKey(key, "112233", EmailVerificationPurpose)

	assert.True(t, VerifyCodeWithKey(key, "112233", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, "000000", EmailVerificationPurpose))

	// 一次性消费：成功一次后即失效（防重放）。
	assert.True(t, VerifyCodeWithKeyConsume(key, "112233", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKeyConsume(key, "112233", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, "112233", EmailVerificationPurpose))

	// 删除后失效。
	RegisterVerificationCodeWithKey(key, "998877", EmailVerificationPurpose)
	DeleteKey(key, EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey(key, "998877", EmailVerificationPurpose))

	// 过期：把有效期临时置 0，注册后立即过期。
	prevValid := VerificationValidMinutes
	VerificationValidMinutes = 0
	RegisterVerificationCodeWithKey(key, "445566", EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey(key, "445566", EmailVerificationPurpose))
	VerificationValidMinutes = prevValid
}

// redisTestClient 在 TEST_REDIS_URL（默认 redis://127.0.0.1:6379/1）上构建客户端；
// 不可用时返回 nil（测试跳过 Redis 路径）。
func redisTestClient(t *testing.T) *redis.Client {
	t.Helper()
	raw := "redis://127.0.0.1:6379/1"
	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		raw = v
	}
	opt, err := redis.ParseURL(raw)
	require.NoError(t, err)
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skip("redis not available, skipping Redis verification path: " + err.Error())
	}
	return client
}

// TestVerificationRedisPath 覆盖 G5：Redis 可用时验证码走 Redis（TTL + 原子消费），
// 语义与内存路径一致。
func TestVerificationRedisPath(t *testing.T) {
	client := redisTestClient(t)
	prevEnabled, prevRDB := RedisEnabled, RDB
	RedisEnabled, RDB = true, client
	defer func() {
		RedisEnabled, RDB = prevEnabled, prevRDB
	}()

	key := "redis-path-" + GenerateVerificationCode(6) + "@example.com"
	RegisterVerificationCodeWithKey(key, "556677", EmailVerificationPurpose)

	// 校验成功；错误码校验失败（不消费）。
	assert.True(t, VerifyCodeWithKey(key, "556677", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, "000000", EmailVerificationPurpose))
	// 一次性消费防重放。
	assert.True(t, VerifyCodeWithKeyConsume(key, "556677", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKeyConsume(key, "556677", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, "556677", EmailVerificationPurpose))

	// 删除后失效。
	RegisterVerificationCodeWithKey(key, "223344", EmailVerificationPurpose)
	DeleteKey(key, EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey(key, "223344", EmailVerificationPurpose))

	// TTL 已按有效期设置（不等待过期，直接断言 key 带过期时间）。
	RegisterVerificationCodeWithKey(key, "778899", EmailVerificationPurpose)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ttl, err := client.TTL(ctx, verificationRedisPrefix+EmailVerificationPurpose+key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	DeleteKey(key, EmailVerificationPurpose)
}

// TestVerificationRedisMismatchDoesNotConsume 覆盖 P1-2/N2：Redis 命中但码错
// （Lua 返回 mismatch）时必须返回 false 且不得消费（防双重消费/误判），
// 正确码随后仍能一次性消费。
func TestVerificationRedisMismatchDoesNotConsume(t *testing.T) {
	client := redisTestClient(t)
	prevEnabled, prevRDB := RedisEnabled, RDB
	RedisEnabled, RDB = true, client
	defer func() {
		RedisEnabled, RDB = prevEnabled, prevRDB
	}()

	key := "redis-mismatch-" + GenerateVerificationCode(6) + "@example.com"
	RegisterVerificationCodeWithKey(key, "246810", EmailVerificationPurpose)

	// 错误码：Redis 中命中但值不匹配 → false，且不消费。
	assert.False(t, VerifyCodeWithKeyConsume(key, "000000", EmailVerificationPurpose))
	// 正确码仍可用（mismatch 未消费）→ 一次性消费成功 → 重放失败。
	assert.True(t, VerifyCodeWithKeyConsume(key, "246810", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKeyConsume(key, "246810", EmailVerificationPurpose))
	DeleteKey(key, EmailVerificationPurpose)
}

// TestVerificationRedisDownFallsBackToMemory 覆盖 P1-2/N2：Redis 故障（SET/GET/Lua
// 全部失败）时注册落入内存，校验/消费回读内存兜底且仍保持一次性语义。
func TestVerificationRedisDownFallsBackToMemory(t *testing.T) {
	dead, err := redis.ParseURL("redis://127.0.0.1:1/1") // 无服务监听 → 连接立即拒绝
	require.NoError(t, err)
	client := redis.NewClient(dead)
	prevEnabled, prevRDB := RedisEnabled, RDB
	RedisEnabled, RDB = true, client
	defer func() {
		RedisEnabled, RDB = prevEnabled, prevRDB
		_ = client.Close()
	}()

	key := "redis-down-" + GenerateVerificationCode(6) + "@example.com"
	RegisterVerificationCodeWithKey(key, "135790", EmailVerificationPurpose) // SET 失败 → 落内存

	assert.True(t, VerifyCodeWithKey(key, "135790", EmailVerificationPurpose))       // GET 失败 → 双读内存
	assert.True(t, VerifyCodeWithKeyConsume(key, "135790", EmailVerificationPurpose)) // Lua 失败 → 内存消费
	assert.False(t, VerifyCodeWithKeyConsume(key, "135790", EmailVerificationPurpose)) // 一次性
	assert.False(t, VerifyCodeWithKey(key, "135790", EmailVerificationPurpose))        // 已消费
}