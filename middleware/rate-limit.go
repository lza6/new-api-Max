package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

const redisRateLimitNamespace = "rateLimit:v2"

// Redis rate limiting intentionally uses a fixed window. The single Lua script
// makes increment, expiry, and the limit decision atomic, while retaining the
// simple fixed-window behavior: traffic at a window boundary can burst up to
// twice the configured limit. Do not replace this with a sliding-window ZSET
// unless that externally visible behavior is intentionally changed.
const redisFixedWindowScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  ttl = redis.call('TTL', KEYS[1])
end
if count > tonumber(ARGV[1]) then
  return {0, count, ttl}
end
return {1, count, ttl}
`

var inMemoryRateLimiter common.InMemoryRateLimiter

var defNext = func(c *gin.Context) {
	c.Next()
}

func redisIPRateLimitKey(mark string, clientIP string) string {
	return fmt.Sprintf("%s:ip:%s:%s", redisRateLimitNamespace, mark, clientIP)
}

func redisUserRateLimitKey(mark string, userID int) string {
	return fmt.Sprintf("%s:user:%s:%d", redisRateLimitNamespace, mark, userID)
}

func redisReplyInteger(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer reply type %T", value)
	}
}

func redisFixedWindowTake(ctx context.Context, key string, maxRequestNum int, duration int64) (bool, int64, int64, error) {
	if common.RDB == nil {
		return false, 0, 0, errors.New("Redis client is not initialized")
	}
	if key == "" {
		return false, 0, 0, errors.New("rate limit key is empty")
	}
	if maxRequestNum <= 0 {
		return false, 0, 0, errors.New("rate limit maximum must be positive")
	}
	if duration <= 0 {
		return false, 0, 0, errors.New("rate limit duration must be positive")
	}

	values, err := common.RDB.Eval(
		ctx,
		redisFixedWindowScript,
		[]string{key},
		maxRequestNum,
		duration,
	).Slice()
	if err != nil {
		return false, 0, 0, err
	}
	if len(values) != 3 {
		return false, 0, 0, fmt.Errorf("unexpected Redis rate limit reply length %d", len(values))
	}

	allowedValue, err := redisReplyInteger(values[0])
	if err != nil {
		return false, 0, 0, err
	}
	count, err := redisReplyInteger(values[1])
	if err != nil {
		return false, 0, 0, err
	}
	ttlSeconds, err := redisReplyInteger(values[2])
	if err != nil {
		return false, 0, 0, err
	}

	return allowedValue == 1, count, ttlSeconds, nil
}

// redisRateLimiter 按客户端 IP 限流。Redis 不可用（实例故障、OOM、网络抖动）
// 时降级为本进程内存限流而非 fail-closed 500：fail-closed 会把一次缓存故障
// 放大成全站 5xx——2026-09-29 生产事故中 Redis OOM 导致限流 EVAL 全部失败，
// 246 次请求被直接转成 500，/api/status 变慢后上游被 Caddy 摘除，全站 503。
// 降级保留限流效果（内存桶与 Redis 桶参数一致），只在跨实例部署时精度下降。
func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	allowed, _, ttlSeconds, err := redisFixedWindowTake(
		c.Request.Context(),
		redisIPRateLimitKey(mark, c.ClientIP()),
		maxRequestNum,
		duration,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("rate limit check failed (mark=%s), falling back to in-memory limiter: %v", mark, err))
		memoryRateLimiter(c, maxRequestNum, duration, mark)
		return
	}
	if !allowed {
		writeRateLimited(c, ttlSeconds)
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		writeRateLimited(c, duration)
		return
	}
}

// writeRateLimited rejects the request with 429 and a Retry-After hint so
// clients can back off instead of treating the rejection as a fatal error.
// The in-memory limiter cannot report the remaining window, so callers
// without a TTL pass the full window duration as a conservative upper bound.
// B6-2：统一携带机器可读 error.type=rate_limited 供前端人话映射。
func writeRateLimited(c *gin.Context, retryAfterSeconds int64) {
	// §4.1.4 指标：限流命中计数。
	common.MetricsInc("rate_limit_hits_total", map[string]string{"type": "http_429"}, 1)
	if retryAfterSeconds > 0 {
		c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message": "rate limited, please retry later",
			"type":    "rate_limited",
			"code":    "rate_limited",
		},
	})
	c.Abort()
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	// It's safe to call multi times. Keep the fallback ready before requests
	// arrive so a concurrent Redis outage cannot race the in-memory limiter's
	// first initialization.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
	return func(c *gin.Context) {
		memoryRateLimiter(c, maxRequestNum, duration, mark)
	}
}

// PublicReadRateLimit 公开只读端点专用限流（60 次/分钟/IP，宽松）。
// §4.1.5：公开只读聚合端点（/v1/pricing、/v1/stats/subscriptions）不应用
// CriticalRateLimit（20/20min 为敏感写端点设计，会 429 真实访客）；只读数据
// 已由数据层/响应层短缓存兜底，宽松 IP 限流足以防滥用。
// 逃生口：env PUBLIC_READ_RATE_LIMIT_ENABLE=false 可完全关闭（公开只读无鉴权，
// 若需不限流部署）。注意该限流经 Redis 限流器（Redis 不可用时按既有全局模式
// fail-closed 500）；如需无 Redis 依赖，置 false。
func PublicReadRateLimit() func(c *gin.Context) {
	if strings.EqualFold(os.Getenv("PUBLIC_READ_RATE_LIMIT_ENABLE"), "false") {
		return func(c *gin.Context) { c.Next() }
	}
	return rateLimitFactory(60, 60, "PR")
}

func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		return rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
	}
	return defNext
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	if !common.GlobalApiRateLimitEnable {
		return defNext
	}
	limiter := rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
	return func(c *gin.Context) {
		// [修复防御] 健康检查/探针自伤：Caddy 等 LB 以本机 IP 高频打
		// /api/status(/api/uptime/status) 做活跃健康检查，若计入 GA 全局
		// 限流桶，探针会先打爆桶→429→后端被误判不健康→公网 503。
		// 探活端点仅返回静态/低开销状态，无鉴权风险面，豁免不计费。
		if c.Request.Method == http.MethodGet && isHealthProbePath(c.Request.URL.Path) {
			c.Next()
			return
		}
		limiter(c)
	}
}

// isHealthProbePath 判定是否 LB 健康检查探针路径（只读、低成本、无敏感数据）。
func isHealthProbePath(path string) bool {
	return path == "/api/status" || path == "/api/uptime/status"
}

func CriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
	}
	return defNext
}

// LoginRateLimit 登录接口专属限流（后台可开关，默认关闭；与全局
// CriticalRateLimit 解耦——登录族端点不再被全局敏感端点限流连带）。
// 开启后按 login_rate_limit.num / login_rate_limit.duration 以客户端 IP 限流。
func LoginRateLimit() func(c *gin.Context) {
	if operation_setting.IsLoginRateLimitEnabled() {
		return rateLimitFactory(
			operation_setting.GetLoginRateLimitNum(),
			operation_setting.GetLoginRateLimitDuration(),
			"LG",
		)
	}
	return defNext
}

func UserCriticalRateLimit(scope string) func(c *gin.Context) {
	if !common.CriticalRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(
		common.CriticalRateLimitNum,
		common.CriticalRateLimitDuration,
		"UC:"+scope,
	)
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}

// userRateLimitFactory creates a rate limiter keyed by authenticated user ID
// instead of client IP, making it resistant to proxy rotation attacks.
// Must be used AFTER authentication middleware (UserAuth).
func userRateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	// It's safe to call multi times. Keep the fallback ready before requests
	// arrive so a concurrent Redis outage cannot race the in-memory limiter's
	// first initialization.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	if common.RedisEnabled {
		return func(c *gin.Context) {
			userID := c.GetInt("id")
			if userID == 0 {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			userRedisRateLimiter(c, maxRequestNum, duration, mark, userID)
		}
	}
	return func(c *gin.Context) {
		userID := c.GetInt("id")
		if userID == 0 {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		memoryUserRateLimiter(c, maxRequestNum, duration, mark, userID)
	}
}

// memoryUserRateLimiter 是 userRedisRateLimiter 的进程内存降级实现，
// 与 Redis 桶使用同一 key 语义（mark + userID）与窗口参数。
func memoryUserRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string, userID int) {
	key := fmt.Sprintf("%s:user:%d", mark, userID)
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		writeRateLimited(c, duration)
		return
	}
}

// userRedisRateLimiter is like redisRateLimiter but keyed by authenticated user
// ID (to support user-ID-based keys). Redis 不可用时与 redisRateLimiter 一样
// 降级到进程内存限流，而不是 fail-closed 500（理由见 redisRateLimiter）。
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string, userID int) {
	allowed, _, ttlSeconds, err := redisFixedWindowTake(
		c.Request.Context(),
		redisUserRateLimitKey(mark, userID),
		maxRequestNum,
		duration,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("rate limit check failed (mark=%s, user=%d), falling back to in-memory limiter: %v", mark, userID, err))
		memoryUserRateLimiter(c, maxRequestNum, duration, mark, userID)
		return
	}
	if !allowed {
		writeRateLimited(c, ttlSeconds)
	}
}

// SearchRateLimit returns a per-user rate limiter for search endpoints.
// Configurable via SEARCH_RATE_LIMIT_ENABLE / SEARCH_RATE_LIMIT / SEARCH_RATE_LIMIT_DURATION.
func SearchRateLimit() func(c *gin.Context) {
	if !common.SearchRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(common.SearchRateLimitNum, common.SearchRateLimitDuration, "SR")
}
