package model

import (
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestSubscriptionStatsCache 锁定 §4.1.5：公开订阅统计短缓存——
// TTL 内命中返回同一实例（不触发 DB 聚合）、过期刷新、env=0 关闭直读。
func TestSubscriptionStatsCache(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}, &UserSubscription{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	// ① 开启（60s）：两次调用返回同一实例（缓存命中）。
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "60")
	statsCache.value = nil
	first, err := GetSiteSubscriptionStatsCached()
	require.NoError(t, err)
	second, err := GetSiteSubscriptionStatsCached()
	require.NoError(t, err)
	assert.Same(t, first, second, "TTL 内命中应返回同一缓存实例（不触发 DB 聚合）")

	// ② 关闭（0）：每次重新聚合（不同实例）。
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "0")
	fresh, err := GetSiteSubscriptionStatsCached()
	require.NoError(t, err)
	assert.NotSame(t, first, fresh, "env=0 时每次应实时聚合（不同实例）")

	// ③ 过期：TTL=1s，sleep 后重新聚合。
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "1")
	statsCache.value = nil
	i1, err := GetSiteSubscriptionStatsCached()
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	i2, err := GetSiteSubscriptionStatsCached()
	require.NoError(t, err)
	assert.NotSame(t, i1, i2, "TTL 过期后应重新聚合")
}

// TestSubscriptionStatsCacheInvalidTTL 锁定 §4.1.5：非法/负 env 回退默认 30s，0=关闭。
func TestSubscriptionStatsCacheInvalidTTL(t *testing.T) {
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "not-a-number")
	assert.Equal(t, 30*time.Second, subscriptionStatsCacheTTL())
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "-5")
	assert.Equal(t, 30*time.Second, subscriptionStatsCacheTTL())
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "0")
	assert.Equal(t, time.Duration(0), subscriptionStatsCacheTTL())
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "")
	assert.Equal(t, 30*time.Second, subscriptionStatsCacheTTL())
}

// TestSubscriptionStatsCacheConcurrentMiss 锁定 §4.1.5 P3-1：并发过期 miss 不 panic、
// 双检后同 TTL 内返回一致实例（不重复聚合覆盖）。
func TestSubscriptionStatsCacheConcurrentMiss(t *testing.T) {
	t.Setenv("SUBSCRIPTION_STATS_CACHE_SECONDS", "60")
	statsCache.value = nil
	var wg sync.WaitGroup
	results := make([]*SiteSubscriptionStats, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s, err := GetSiteSubscriptionStatsCached()
			if err == nil {
				results[n] = s
			}
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(results); i++ {
		require.NotNil(t, results[i])
		assert.Same(t, results[0], results[i], "并发 miss 后应收敛到同一缓存实例")
	}
}
