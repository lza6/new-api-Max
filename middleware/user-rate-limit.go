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
//
// 订阅用户例外：持有 active 订阅且套餐档位 > 0 时，跳过基础默认（含分组/用户
// 覆盖经本中间件的叠加），由订阅档位中间件（套餐档位/管理员对单个订阅的覆盖）
// 统一约束。否则基础 RPM(120) 会先于订阅 RPM(150) 触发，订阅档位形同虚设。

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
		if subConcurrency, subRpm, hasSub := resolveSubscriptionTier(userId); hasSub &&
			(subConcurrency > 0 || subRpm > 0) {
			// 订阅档位已约束该用户（并发/RPM），基础默认不再叠加。
			c.Next()
			return
		}
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
