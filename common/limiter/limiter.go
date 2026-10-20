package limiter

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/go-redis/redis/v8"
)

//go:embed lua/rate_limit.lua
var rateLimitScript string

// rateLimitLua 用 Script.Run 执行：go-redis 在 EVALSHA 收到 NOSCRIPT 时会自动
// 回退到 EVAL 并重新缓存。**不要**再手动 ScriptLoad + EvalSha：
// Redis 重启、故障切换或 SCRIPT FLUSH 后脚本缓存为空，EvalSha 会持续返回
// NOSCRIPT，而中间件把它转成 500 —— 表现为“Redis 重启后所有 /v1 请求 500”。
var rateLimitLua = redis.NewScript(rateLimitScript)

type RedisLimiter struct {
	client *redis.Client
}

// New 返回绑定到该客户端的限流器。
//
// **不要用 sync.Once 做单例**：调用方可能在不同生命周期拿到不同 client
// （测试、重连、多实例嵌入），单例会把旧 client 永久固化。
func New(_ context.Context, r *redis.Client) *RedisLimiter {
	return &RedisLimiter{client: r}
}

func (rl *RedisLimiter) Allow(ctx context.Context, key string, opts ...Option) (bool, error) {
	// 默认配置
	config := &Config{
		Capacity:  10,
		Rate:      1,
		Requested: 1,
	}

	// 应用选项模式
	for _, opt := range opts {
		opt(config)
	}

	// 执行限流（Script.Run 自带 NOSCRIPT 回退）
	result, err := rateLimitLua.Run(
		ctx,
		rl.client,
		[]string{key},
		config.Requested,
		config.Rate,
		config.Capacity,
	).Int()

	if err != nil {
		return false, fmt.Errorf("rate limit failed: %w", err)
	}
	return result == 1, nil
}

// Config 配置选项模式
type Config struct {
	Capacity  int64
	Rate      int64
	Requested int64
}

type Option func(*Config)

func WithCapacity(c int64) Option {
	return func(cfg *Config) { cfg.Capacity = c }
}

func WithRate(r int64) Option {
	return func(cfg *Config) { cfg.Rate = r }
}

func WithRequested(n int64) Option {
	return func(cfg *Config) { cfg.Requested = n }
}
