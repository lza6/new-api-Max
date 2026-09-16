package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"

	"github.com/gin-gonic/gin"
)

// T5 单密钥限速（rpm / qbs / concurrency）。
//
// 设计：完全独立于既有的 ModelRequestRateLimit（按用户 + 按分组）。本中间件
// 只在该密钥配置了 rate_limit 时生效，三种维度叠加：
//   - rpm：滑动窗口（InMemoryRateLimiter），key = token:<id>:rpm，窗口 60s
//   - qbs：令牌桶按秒补充，key = token:<id>:qbs，容量=qbs，每秒回填 qbs
//   - concurrency：并发信号量（进程内计数），请求进入 acquire，结束 release，
//     超过上限返回 429。
//
// 单一实例部署下用进程内计数即可；多实例需接 Redis（本期不做，文档注明）。

var tokenRateLimitRpmLimiter = &common.InMemoryRateLimiter{}

func init() {
	tokenRateLimitRpmLimiter.Init(time.Minute)
}

// tokenConcurrencyStore 记录每个 token 当前进行中的请求数。
var tokenConcurrencyStore = struct {
	sync.Mutex
	counts map[int]int
}{counts: make(map[int]int)}

func acquireTokenConcurrency(tokenId int, max int) bool {
	if max <= 0 {
		return true
	}
	tokenConcurrencyStore.Lock()
	defer tokenConcurrencyStore.Unlock()
	if tokenConcurrencyStore.counts[tokenId] >= max {
		return false
	}
	tokenConcurrencyStore.counts[tokenId]++
	return true
}

func releaseTokenConcurrency(tokenId int) {
	tokenConcurrencyStore.Lock()
	defer tokenConcurrencyStore.Unlock()
	if v := tokenConcurrencyStore.counts[tokenId]; v > 1 {
		tokenConcurrencyStore.counts[tokenId] = v - 1
	} else {
		delete(tokenConcurrencyStore.counts, tokenId)
	}
}

// tokenQBSBuckets 令牌桶：tokenId -> 剩余令牌 + 上次回填时间。
var tokenQBSBuckets = struct {
	sync.Mutex
	tokens map[int]struct {
		remaining float64
		updated   time.Time
	}
}{tokens: make(map[int]struct {
	remaining float64
	updated   time.Time
})}

func allowTokenQBS(tokenId, qbs int) bool {
	if qbs <= 0 {
		return true
	}
	now := time.Now()
	tokenQBSBuckets.Lock()
	defer tokenQBSBuckets.Unlock()
	b, ok := tokenQBSBuckets.tokens[tokenId]
	if !ok {
		b = struct {
			remaining float64
			updated   time.Time
		}{remaining: float64(qbs), updated: now}
	}
	// 按经过的时间回填令牌（每秒 qbs 个，容量不超过 qbs）。
	elapsed := now.Sub(b.updated).Seconds()
	if elapsed > 0 {
		b.remaining += elapsed * float64(qbs)
		if b.remaining > float64(qbs) {
			b.remaining = float64(qbs)
		}
		b.updated = now
	}
	if b.remaining < 1 {
		tokenQBSBuckets.tokens[tokenId] = b
		return false
	}
	b.remaining--
	tokenQBSBuckets.tokens[tokenId] = b
	return true
}

func parseTokenRateLimitConfig(c *gin.Context) model.TokenRateLimitConfig {
	return parseTokenRateLimitConfigFromString(common.GetContextKeyString(c, constant.ContextKeyTokenRateLimit))
}

func parseTokenRateLimitConfigFromString(raw string) model.TokenRateLimitConfig {
	if raw == "" {
		return model.TokenRateLimitConfig{}
	}
	var cfg model.TokenRateLimitConfig
	if err := common.UnmarshalJsonStr(raw, &cfg); err != nil {
		return model.TokenRateLimitConfig{}
	}
	return cfg
}

// TokenRateLimit T5 单密钥限速中间件：rpm / qbs / concurrency。
// 未配置 rate_limit 的 token 直接放行；超限返回 429。
func TokenRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := parseTokenRateLimitConfig(c)
		if cfg.RPM <= 0 && cfg.QBS <= 0 && cfg.Concurrency <= 0 {
			c.Next()
			return
		}

		tokenId := c.GetInt("token_id")

		// rpm：60 秒滑动窗口
		if cfg.RPM > 0 {
			key := "token:" + strconv.Itoa(tokenId) + ":rpm"
			if !tokenRateLimitRpmLimiter.Request(key, cfg.RPM, 60) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, "您已达到该密钥的请求速率限制（每分钟 "+strconv.Itoa(cfg.RPM)+" 次）")
				return
			}
		}

		// qbs：令牌桶按秒
		if cfg.QBS > 0 && !allowTokenQBS(tokenId, cfg.QBS) {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, "您已达到该密钥的请求配额限制（每秒 "+strconv.Itoa(cfg.QBS)+" 次）")
			return
		}

		// concurrency：并发信号量
		if cfg.Concurrency > 0 {
			if !acquireTokenConcurrency(tokenId, cfg.Concurrency) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, "该密钥并发请求数已达上限（"+strconv.Itoa(cfg.Concurrency)+"）")
				return
			}
			defer releaseTokenConcurrency(tokenId)
		}

		c.Next()
	}
}
