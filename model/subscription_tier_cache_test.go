package model

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetActiveSubCache 清空包级正缓存与 TTL 开关，避免用例间污染。
func resetActiveSubCache() {
	activeSubCache.Lock()
	defer activeSubCache.Unlock()
	activeSubCache.m = make(map[int]cachedActiveSubscriptions)
}

func sampleSummaries() []SubscriptionSummary {
	return []SubscriptionSummary{
		{
			Subscription: &UserSubscription{Id: 901, UserId: 63001, PlanId: 1, Status: "active", EndTime: time.Now().Add(time.Hour).Unix()},
		},
	}
}

func TestSubscriptionActiveCacheEnableDisable(t *testing.T) {
	resetActiveSubCache()
	t.Run("unset uses default 10s", func(t *testing.T) {
		t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "")
		assert.True(t, IsSubscriptionActiveCacheEnabled())
		assert.Equal(t, 10*time.Second, getSubscriptionActiveCacheTTL())
	})
	t.Run("zero disables", func(t *testing.T) {
		t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "0")
		assert.False(t, IsSubscriptionActiveCacheEnabled())
		assert.Equal(t, time.Duration(0), getSubscriptionActiveCacheTTL())
	})
	t.Run("invalid falls back to default", func(t *testing.T) {
		t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "not-a-number")
		assert.Equal(t, 10*time.Second, getSubscriptionActiveCacheTTL())
	})
}

func TestSubscriptionActiveCacheStoreGetInvalidate(t *testing.T) {
	t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "10")
	resetActiveSubCache()

	// 未命中
	_, found := GetCachedActiveSubscriptions(63001)
	require.False(t, found)

	// 命中
	StoreCachedActiveSubscriptions(63001, sampleSummaries())
	got, found := GetCachedActiveSubscriptions(63001)
	require.True(t, found)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Subscription)
	assert.Equal(t, 1, got[0].Subscription.PlanId)

	// 变更失效后未命中
	InvalidateActiveSubscriptionCache(63001)
	_, found = GetCachedActiveSubscriptions(63001)
	assert.False(t, found)

	// 其他用户不受影响
	StoreCachedActiveSubscriptions(63001, sampleSummaries())
	_, found = GetCachedActiveSubscriptions(63002)
	assert.False(t, found)
}

func TestSubscriptionActiveCacheDisabledStoresNothing(t *testing.T) {
	t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "0")
	resetActiveSubCache()

	StoreCachedActiveSubscriptions(63007, sampleSummaries())
	_, found := GetCachedActiveSubscriptions(63007)
	assert.False(t, found)
	// 失效仍安全（幂等）。
	InvalidateActiveSubscriptionCache(63007)
}

func TestSubscriptionActiveCacheExpiry(t *testing.T) {
	t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "1")
	resetActiveSubCache()

	StoreCachedActiveSubscriptions(63009, sampleSummaries())
	_, found := GetCachedActiveSubscriptions(63009)
	require.True(t, found)

	time.Sleep(1100 * time.Millisecond)
	_, found = GetCachedActiveSubscriptions(63009)
	assert.False(t, found, "缓存应在 TTL 后过期")
}

func TestSubscriptionActiveCacheConcurrentAccess(t *testing.T) {
	t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "10")
	resetActiveSubCache()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			uid := 64000 + n%10
			StoreCachedActiveSubscriptions(uid, sampleSummaries())
			_, _ = GetCachedActiveSubscriptions(uid)
			if n%5 == 0 {
				InvalidateActiveSubscriptionCache(uid)
			}
		}(i)
	}
	wg.Wait()
	// 并发读写不 panic；缓存仍在正常提供服务。
	StoreCachedActiveSubscriptions(64080, sampleSummaries())
	_, found := GetCachedActiveSubscriptions(64080)
	assert.True(t, found)
}

func TestSubscriptionActiveCacheCapEvictsExpiredNotAll(t *testing.T) {
	t.Setenv("SUBSCRIPTION_ACTIVE_CACHE_SECONDS", "60") // 长 TTL，制造大量存活条目
	resetActiveSubCache()

	// ① 灌满 cap+1 → 触发容量淘汰：不整表清空（保留部分热点），且不超过上限。
	for i := 0; i < subscriptionActiveCacheMaxEntries+1; i++ {
		StoreCachedActiveSubscriptions(70000+i, sampleSummaries())
	}
	activeSubCache.Lock()
	size := len(activeSubCache.m)
	activeSubCache.Unlock()
	assert.LessOrEqual(t, size, subscriptionActiveCacheMaxEntries)
	assert.Greater(t, size, 0)

	// ② 把残留条目全部标为过期。
	activeSubCache.Lock()
	for k, e := range activeSubCache.m {
		activeSubCache.m[k] = cachedActiveSubscriptions{summaries: e.summaries, expiresAt: time.Now().Add(-time.Second)}
	}
	activeSubCache.Unlock()

	// ③ 再次灌满触发容量淘汰：过期条目应被清走，新条目可读。
	for i := 0; i < subscriptionActiveCacheMaxEntries+1; i++ {
		StoreCachedActiveSubscriptions(80000+i, sampleSummaries())
	}
	activeSubCache.Lock()
	expiredRemaining := 0
	for _, e := range activeSubCache.m {
		if time.Now().After(e.expiresAt) {
			expiredRemaining++
		}
	}
	size2 := len(activeSubCache.m)
	activeSubCache.Unlock()
	assert.Zero(t, expiredRemaining, "过期条目应在容量淘汰时被清走")
	assert.LessOrEqual(t, size2, subscriptionActiveCacheMaxEntries)
}
