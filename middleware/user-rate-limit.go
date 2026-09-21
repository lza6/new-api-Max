package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/setting/relay_setting"

	"github.com/gin-gonic/gin"
)

// UserRateLimit T7 每用户基础限速：并发（请求/秒）+ RPM，超限 429。
//
// 生效档位：用户覆盖 > 分组覆盖 > 基础默认（管理员在 relay 设置中调控）。
// 与订阅档位（SubscriptionRateLimit）、单密钥限速（TokenRateLimit）并存，
// 任一超限即 429（取最严，不绕过既有防线）。进程内计数，多实例需接 Redis。

var userRateLimitRpmLimiter = &common.InMemoryRateLimiter{}

func init() {
	userRateLimitRpmLimiter.Init(time.Minute)
}

// userRateLimitConcurrencyStore 记录每个用户当前进行中的请求数。
var userRateLimitConcurrencyStore = struct {
	sync.Mutex
	counts map[int]int
}{counts: make(map[int]int)}

func acquireUserRateLimitConcurrency(userId int, max int) bool {
	if max <= 0 {
		return true
	}
	userRateLimitConcurrencyStore.Lock()
	defer userRateLimitConcurrencyStore.Unlock()
	if userRateLimitConcurrencyStore.counts[userId] >= max {
		return false
	}
	userRateLimitConcurrencyStore.counts[userId]++
	return true
}

func releaseUserRateLimitConcurrency(userId int) {
	userRateLimitConcurrencyStore.Lock()
	defer userRateLimitConcurrencyStore.Unlock()
	if v := userRateLimitConcurrencyStore.counts[userId]; v > 1 {
		userRateLimitConcurrencyStore.counts[userId] = v - 1
	} else {
		delete(userRateLimitConcurrencyStore.counts, userId)
	}
}

// UserRateLimit 每用户基础限速中间件：rpm（60s 滑动窗口）+ 并发（进程内信号量）。
func UserRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}
		concurrency, rpm := relay_setting.GetUserRateLimitTier(userId, group)
		if concurrency <= 0 && rpm <= 0 {
			c.Next()
			return
		}

		if rpm > 0 {
			key := "ubase:" + strconv.Itoa(userId) + ":rpm"
			if !userRateLimitRpmLimiter.Request(key, rpm, 60) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests,
					"基础请求速率已达上限（每分钟 "+strconv.Itoa(rpm)+" 次）")
				return
			}
		}

		if concurrency > 0 {
			if !acquireUserRateLimitConcurrency(userId, concurrency) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests,
					"基础并发请求已达上限（每秒 "+strconv.Itoa(concurrency)+" 次）")
				return
			}
			defer releaseUserRateLimitConcurrency(userId)
		}

		c.Next()
	}
}
