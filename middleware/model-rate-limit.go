package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/common/limiter"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	// 成功计数键的版本后缀。旧实现把成功计数存成 Redis LIST（LPUSH/LTRIM），
	// 新实现改用 ZSET（按时间戳滑动窗口、可精确退还）。键名必须升级，
	// 否则在旧 LIST 键上跑 ZADD 会 WRONGTYPE，让所有请求 500。
	modelSuccessLimitKeyVersion = "v2"
)

// 成功计数槽位序号：保证同一纳秒内并发的预留仍有唯一 member。
var modelSuccessSlotSeq atomic.Uint64

// reserveSuccessSlotScript 原子地「清理过期 → 判限额 → 预留一个槽位」。
//
// 旧实现是 LLen 判断 + 请求后 LPUSH 记录，两者分开执行：并发请求会同时看到
// length < max 而全部放行，实际成功数可以轻易超过上限（也就是「限流形同虚设」）。
// Lua 在 Redis 单线程里完成判定与占位，判限额与占位之间没有可插入的窗口。
//
// KEYS[1] 成功计数 ZSET
// ARGV[1] 当前毫秒时间戳  ARGV[2] 窗口毫秒  ARGV[3] 上限  ARGV[4] 本次 member
var reserveSuccessSlotScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local maxCount = tonumber(ARGV[3])
local member = ARGV[4]
if window > 0 then
  redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)
end
if maxCount > 0 and redis.call('ZCARD', key) >= maxCount then
  if window > 0 then redis.call('PEXPIRE', key, window) end
  return 0
end
redis.call('ZADD', key, now, member)
if window > 0 then redis.call('PEXPIRE', key, window) end
return 1
`)

// refundSuccessSlotScript 退还一个预留槽位（请求最终失败时调用）。
var refundSuccessSlotScript = redis.NewScript(`
return redis.call('ZREM', KEYS[1], ARGV[1])
`)

// reserveRedisSuccessSlot 预占一个成功请求名额。返回的 member 用于失败时退还。
func reserveRedisSuccessSlot(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (string, bool, error) {
	if maxCount <= 0 || duration <= 0 {
		// 未配置上限或窗口无效 → 不做成功数限制（与旧实现一致）。
		return "", true, nil
	}
	member := fmt.Sprintf("%d-%d", time.Now().UnixNano(), modelSuccessSlotSeq.Add(1))
	windowMs := duration * 1000
	reserved, err := reserveSuccessSlotScript.Run(ctx, rdb, []string{key},
		time.Now().UnixMilli(), windowMs, maxCount, member).Int()
	if err != nil {
		return "", false, err
	}
	return member, reserved == 1, nil
}

// refundRedisSuccessSlot 归还预留；失败只记日志，不能影响请求结果。
func refundRedisSuccessSlot(ctx context.Context, rdb *redis.Client, key, member string) {
	if member == "" {
		return
	}
	if err := refundSuccessSlotScript.Run(ctx, rdb, []string{key}, member).Err(); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("model rate limit: refund success slot failed key=%s err=%v", key, err))
	}
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int, group string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		// 1. 原子预占成功名额。
		//    计数按分组隔离：不同分组有各自的限额，共用一个 key 会让 A 组挤占 B 组额度。
		//    **必须先占位再处理请求**：若等请求成功后再记录，并发请求会同时通过
		//    「未达上限」的判断，成功数可以超过限额（旧实现的真实缺陷）。
		successKey := fmt.Sprintf("rateLimit:%s:%s:%s:%s", ModelRequestRateLimitSuccessCountMark, modelSuccessLimitKeyVersion, group, userId)
		member, allowed, err := reserveRedisSuccessSlot(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("model rate limit: success reservation failed user=%s group=%s err=%v", userId, group, err))
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}
		// 被总请求限流拒绝的请求不应占用成功名额（旧顺序也是如此）。
		refundOnExit := true
		defer func() {
			// panic 也要退还，否则槽位会一直占到这个窗口结束。
			if r := recover(); r != nil {
				refundRedisSuccessSlot(ctx, rdb, successKey, member)
				panic(r)
			}
			if refundOnExit {
				refundRedisSuccessSlot(ctx, rdb, successKey, member)
			}
		}()

		// 2. 检查总请求数限制（含失败次数；令牌桶）。
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:total:%s:%s", group, userId)
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(rateLimitCapacity(totalMaxCount, duration)),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("model rate limit: total check failed user=%s group=%s err=%v", userId, group, err))
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
				return
			}
		}

		// 3. 处理请求
		c.Next()

		// 4. 成功才保留名额；失败请求退还。
		refundOnExit = c.Writer.Status() >= 400
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int, group string) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + group + ":" + userId
		successKey := ModelRequestRateLimitSuccessCountMark + modelSuccessLimitKeyVersion + group + ":" + userId

		// 1. 原子预占成功名额（与 Redis 路径同语义：先占位，失败/被总限流拒绝再退还）。
		reserved := false
		if successMaxCount > 0 && duration > 0 {
			if !inMemoryRateLimiter.Request(successKey, successMaxCount, duration) {
				c.Status(http.StatusTooManyRequests)
				c.Abort()
				return
			}
			reserved = true
		}
		refundOnExit := reserved
		defer func() {
			if r := recover(); r != nil {
				if refundOnExit {
					inMemoryRateLimiter.Refund(successKey)
				}
				panic(r)
			}
			if refundOnExit {
				inMemoryRateLimiter.Refund(successKey)
			}
		}()

		// 2. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 成功才保留名额；失败请求退还。
		refundOnExit = reserved && c.Writer.Status() >= 400
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if isRateLimitExemptModel(c) {
			c.Next()
			return
		}

		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := rateLimitDurationSeconds(setting.ModelRequestRateLimitDurationMinutes)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount, group)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount, group)(c)
		}
	}
}

func rateLimitDurationSeconds(durationMinutes int) int64 {
	if durationMinutes <= 0 {
		return 0
	}
	minutes := int64(durationMinutes)
	if minutes > math.MaxInt64/60 {
		return math.MaxInt64
	}
	return minutes * 60
}

func rateLimitCapacity(count int, durationSeconds int64) int64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	c := int64(count)
	if c > math.MaxInt64/durationSeconds {
		return math.MaxInt64
	}
	return c * durationSeconds
}
