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