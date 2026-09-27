package model

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// 订阅档位正向缓存（§4.1.1 热路径缓存 · 真实缺口定位）。
//
// 现状证据：limit 中间件 subscription-rate-limit.go 的 resolveSubscriptionTier 对
// 「存在 active 订阅」的用户**每个请求**执行一次 GetAllActiveUserSubscriptions DB 查询
// （无订阅用户已有 service 负缓存 15s 兜底）。该查询有复合索引但仍是每请求一次 DB 往返。
//
// 设计（只缓存纯软限数据，不碰计费）：
//   - 仅缓存「active 订阅摘要」，供限流中间件计算并发/RPM 档位（EffectiveTier）。
//   - 计费路径（NewBillingSession 的 HasActiveUserSubscription / AllowWalletOverflow、
//     PreConsumeUserSubscription）**不读本缓存**，保持 DB 权威。
//   - 短 TTL（默认 10s）+ 订阅变更写后失效（创建/过期/重置/降级/删除），把过期窗口压到
//     秒级——对限流档位是软约束，秒级滞后可接受。
//   - 开关：env SUBSCRIPTION_ACTIVE_CACHE_SECONDS，默认 10；0 = 关闭（行为与基线完全一致，
//     每请求回 FallbackDB）。负缓存（service.CacheNoSubscription）不受影响。

const subscriptionActiveCacheKeyPrefix = "sub_tier"
const subscriptionActiveCacheDefaultTTL = 10 * time.Second
// subscriptionActiveCacheMaxEntries 缓存条目上限；超过时先淘汰过期项，
// 仍超限则随机删除至上限一半（避免整表清空导致高频用户全部缓存抖动）。
const subscriptionActiveCacheMaxEntries = 20000

// getSubscriptionActiveCacheTTL 返回 TTL；<=0（env 置 0）表示关闭缓存。
func getSubscriptionActiveCacheTTL() time.Duration {
	raw := os.Getenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS")
	if raw == "" {
		return subscriptionActiveCacheDefaultTTL
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 0 {
		return subscriptionActiveCacheDefaultTTL
	}
	if sec == 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

// IsSubscriptionActiveCacheEnabled 缓存开关（TTL>0 且 Redis 可用？）。
// 本缓存使用进程内 map（与 limit 中间件「进程内计数」口径一致，多实例各自缓存；
// 失效窗口=TTL，接受与 no-subscription 负缓存相同的多实例局限）。
func IsSubscriptionActiveCacheEnabled() bool {
	return getSubscriptionActiveCacheTTL() > 0
}

type cachedActiveSubscriptions struct {
	summaries []SubscriptionSummary
	expiresAt time.Time
}

var activeSubCache = struct {
	sync.Mutex
	m map[int]cachedActiveSubscriptions
}{m: make(map[int]cachedActiveSubscriptions)}

// GetCachedActiveSubscriptions 返回缓存的 active 订阅摘要；未命中/过期/关闭返回 found=false。
// 返回的切片为内部副本的只读视图，调用方不应修改其元素（避免污染缓存）。
func GetCachedActiveSubscriptions(userId int) ([]SubscriptionSummary, bool) {
	if !IsSubscriptionActiveCacheEnabled() || userId <= 0 {
		return nil, false
	}
	activeSubCache.Lock()
	defer activeSubCache.Unlock()
	entry, ok := activeSubCache.m[userId]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(activeSubCache.m, userId)
		return nil, false
	}
	return entry.summaries, true
}

// StoreCachedActiveSubscriptions 写入 active 订阅摘要缓存（TTL 由开关决定）。
// 空摘要不入缓存（与"无订阅→负缓存"分工明确，避免 found-but-empty 跳过负缓存路径）。
func StoreCachedActiveSubscriptions(userId int, summaries []SubscriptionSummary) {
	if !IsSubscriptionActiveCacheEnabled() || userId <= 0 || len(summaries) == 0 {
		return
	}
	ttl := getSubscriptionActiveCacheTTL()
	if ttl <= 0 {
		return
	}
	activeSubCache.Lock()
	defer activeSubCache.Unlock()
	// 达到上限前先淘汰（>= 保证缓存严格不超过上限），先清过期项，仍超限随机删至一半。
	if len(activeSubCache.m) >= subscriptionActiveCacheMaxEntries {
		now := time.Now()
		for k, e := range activeSubCache.m {
			if now.After(e.expiresAt) {
				delete(activeSubCache.m, k)
			}
		}
		// 仍达上限（低频长 TTL 存活条目多）时随机删至上限一半：部分条目退化到直查 DB，
		// 但避免整表清空造成高频用户全部缓存抖动（非 LRU，仅容量兜底）。
		if len(activeSubCache.m) >= subscriptionActiveCacheMaxEntries {
			for k := range activeSubCache.m {
				delete(activeSubCache.m, k)
				if len(activeSubCache.m) <= subscriptionActiveCacheMaxEntries/2 {
					break
				}
			}
		}
	}
	activeSubCache.m[userId] = cachedActiveSubscriptions{
		summaries: append([]SubscriptionSummary(nil), summaries...),
		expiresAt: time.Now().Add(ttl),
	}
}

// InvalidateActiveSubscriptionCache 订阅变更（创建/过期/重置/降级/删除）时清除正向缓存。
// 在 model 内的全部订阅变更函数调用，避免 model→service 循环依赖。
func InvalidateActiveSubscriptionCache(userId int) {
	if userId <= 0 {
		return
	}
	activeSubCache.Lock()
	defer activeSubCache.Unlock()
	delete(activeSubCache.m, userId)
}