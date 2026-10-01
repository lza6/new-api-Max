package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"

	"github.com/gin-gonic/gin"
)

// SubscriptionRateLimit 订阅档位限流：并发 + RPM，超限返回 429。
//
// 仅对「存在 active 订阅」的用户生效；未订阅用户直接放行（沿用既有
// token/分组/全局限流，不绕过既有防线）。档位来源：套餐自带
// ConcurrencyLimit/RpmLimit，管理员对单个订阅的覆盖（RpmOverride /
// ConcurrencyOverride）优先。进程内计数，单一实例有效；多实例需接
// Redis（与 T5/T6 相同口径，文档注明）。

var subscriptionRpmLimiter = &common.InMemoryRateLimiter{}

func init() {
	subscriptionRpmLimiter.Init(time.Minute)
}

// subscriptionConcurrencyStore 记录每个用户当前进行中的请求数。
var subscriptionConcurrencyStore = struct {
	sync.Mutex
	counts map[int]int
}{counts: make(map[int]int)}

func acquireSubscriptionConcurrency(userId int, max int) bool {
	if max <= 0 {
		return true
	}
	subscriptionConcurrencyStore.Lock()
	defer subscriptionConcurrencyStore.Unlock()
	if subscriptionConcurrencyStore.counts[userId] >= max {
		return false
	}
	subscriptionConcurrencyStore.counts[userId]++
	return true
}

func releaseSubscriptionConcurrency(userId int) {
	subscriptionConcurrencyStore.Lock()
	defer subscriptionConcurrencyStore.Unlock()
	if v := subscriptionConcurrencyStore.counts[userId]; v > 1 {
		subscriptionConcurrencyStore.counts[userId] = v - 1
	} else {
		delete(subscriptionConcurrencyStore.counts, userId)
	}
}

// resolveSubscriptionTier 返回用户当前生效的订阅档位（覆盖优先，0=不限）。
// 无 active 订阅时 hasSub=false。
//
// §4.1.1：订阅用户档位正向缓存（model.GetCachedActiveSubscriptions，短 TTL +
// 订阅变更失效）命中时直接由缓存摘要计算档位，避免每请求一次 DB 查询；
// 未命中才走全部 DB 查询并存回。计费路径不读本缓存（保持 DB 权威）。
func resolveSubscriptionTier(userId int) (concurrencyLimit, rpmLimit int, hasSub bool) {
	if userId <= 0 {
		return 0, 0, false
	}
	if service.HasCachedNoSubscription(userId) {
		return 0, 0, false
	}
	// DB 不可用（启动/关闭窗口或测试环境）时 fail-open：不拦截请求，保证可用性。
	if model.DB == nil {
		return 0, 0, false
	}
	if tiers, found := model.GetCachedActiveSubscriptions(userId); found {
		return resolveFromSubscriptionSummaries(tiers)
	}
	subs, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil || len(subs) == 0 {
		if err == nil {
			service.CacheNoSubscription(userId)
		}
		return 0, 0, false
	}
	model.StoreCachedActiveSubscriptions(userId, subs)
	return resolveFromSubscriptionSummaries(subs)
}

// resolveFromSubscriptionSummaries 从 active 订阅摘要计算档位（与既有语义一致：
// 取 end_time 最新的首条订阅；套餐本身有缓存 GetSubscriptionPlanById）。
func resolveFromSubscriptionSummaries(subs []model.SubscriptionSummary) (concurrencyLimit, rpmLimit int, hasSub bool) {
	if len(subs) == 0 {
		return 0, 0, false
	}
	sub := subs[0].Subscription
	if sub == nil {
		return 0, 0, false
	}
	plan, err := model.GetPlanForSubscription(sub)
	if err != nil || plan == nil {
		return 0, 0, false
	}
	concurrencyLimit, rpmLimit = sub.EffectiveTier(plan.ConcurrencyLimit, plan.RpmLimit)
	return concurrencyLimit, rpmLimit, true
}

// SubscriptionRateLimit 订阅档位限流中间件：rpm（60s 滑动窗口）+ 并发（进程内信号量）。
func SubscriptionRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isRateLimitExemptModel(c) {
			c.Next()
			return
		}

		userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		concurrency, rpm, hasSub := resolveSubscriptionTier(userId)
		if !hasSub || (concurrency <= 0 && rpm <= 0) {
			c.Next()
			return
		}

		if rpm > 0 {
			key := "sub:" + strconv.Itoa(userId) + ":rpm"
			if !subscriptionRpmLimiter.Request(key, rpm, 60) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests,
					"订阅请求速率已达上限（每分钟 "+strconv.Itoa(rpm)+" 次），如需更高 RPM 可联系微信 Tf00798 定制")
				return
			}
		}

		if concurrency > 0 {
			if !acquireSubscriptionConcurrency(userId, concurrency) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests,
					"订阅并发请求已达上限（"+strconv.Itoa(concurrency)+"），如需更高并发可联系微信 Tf00798 定制")
				return
			}
			defer releaseSubscriptionConcurrency(userId)
		}

		c.Next()
	}
}
