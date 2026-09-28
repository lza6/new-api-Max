package model

import (
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// §4.1.5：站点订阅统计响应短缓存（公开只读端点，默认 30s；env
// SUBSCRIPTION_STATS_CACHE_SECONDS=0 可关闭回退实时聚合）。进程内缓存，多实例
// 各自独立（TTL 秒级一致，公开聚合可接受）；过期自动刷新。
var statsCache = struct {
	sync.Mutex
	value    *SiteSubscriptionStats
	expires  time.Time
}{}

func subscriptionStatsCacheTTL() time.Duration {
	raw := os.Getenv("SUBSCRIPTION_STATS_CACHE_SECONDS")
	if raw == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 0 {
		return 30 * time.Second
	}
	return time.Duration(sec) * time.Second
}

// GetSiteSubscriptionStatsCached 返回订阅统计（短缓存命中直接用，未命中实时聚合）。
func GetSiteSubscriptionStatsCached() (*SiteSubscriptionStats, error) {
	ttl := subscriptionStatsCacheTTL()
	if ttl > 0 {
		statsCache.Lock()
		if statsCache.value != nil && time.Now().Before(statsCache.expires) {
			statsCache.Unlock()
			return statsCache.value, nil
		}
		statsCache.Unlock()
	}
	stats, err := GetSiteSubscriptionStats()
	if err != nil {
		return nil, err
	}
	if ttl > 0 {
		statsCache.Lock()
		statsCache.value = stats
		statsCache.expires = time.Now().Add(ttl)
		statsCache.Unlock()
	}
	return stats, nil
}

// T15-A 站点订阅统计（只读聚合，不暴露任何用户/订单明细，仅运营透明度数据）。
// 复用既有 subscription_plans / user_subscriptions 表，无 schema 变更、无新表。

// PlanSubscriptionStat 单个订阅档位的聚合。
type PlanSubscriptionStat struct {
	PlanID int    `json:"plan_id"`
	Title  string `json:"title"`
	Total  int64  `json:"total"`
	Active int64  `json:"active"`
}

// SiteSubscriptionStats 站点订阅运营统计快照。
type SiteSubscriptionStats struct {
	TotalPlans          int                    `json:"total_plans"`
	TotalSubscriptions  int64                  `json:"total_subscriptions"`
	ActiveSubscriptions int64                  `json:"active_subscriptions"`
	ExpiringSoon7d      int64                  `json:"expiring_soon_7d"`
	NewLast30d          int64                  `json:"new_last_30d"`
	ByPlan              []PlanSubscriptionStat `json:"by_plan"`
}

type planStatusCount struct {
	PlanID int    `gorm:"column:plan_id"`
	Status string `gorm:"column:status"`
	Cnt    int64  `gorm:"column:cnt"`
}

// GetSiteSubscriptionStats 计算站点订阅统计。所有查询走 GORM，兼容三库。
func GetSiteSubscriptionStats() (*SiteSubscriptionStats, error) {
	stats := &SiteSubscriptionStats{ByPlan: []PlanSubscriptionStat{}}
	now := common.GetTimestamp()
	weekLater := now + 7*24*60*60
	monthAgo := now - 30*24*60*60

	var totalPlans int64
	if err := DB.Model(&SubscriptionPlan{}).Count(&totalPlans).Error; err != nil {
		return nil, err
	}
	stats.TotalPlans = int(totalPlans)

	if err := DB.Model(&UserSubscription{}).Count(&stats.TotalSubscriptions).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&UserSubscription{}).Where("status = ?", "active").Count(&stats.ActiveSubscriptions).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&UserSubscription{}).
		Where("status = ? AND end_time > ? AND end_time <= ?", "active", now, weekLater).
		Count(&stats.ExpiringSoon7d).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&UserSubscription{}).Where("created_at >= ?", monthAgo).Count(&stats.NewLast30d).Error; err != nil {
		return nil, err
	}

	// 按档位聚合（plan_id + status），再补档位标题。
	var rows []planStatusCount
	if err := DB.Model(&UserSubscription{}).
		Select("plan_id, status, count(*) as cnt").
		Group("plan_id, status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	titles := map[int]string{}
	var plans []SubscriptionPlan
	if err := DB.Select("id", "title").Find(&plans).Error; err != nil {
		return nil, err
	}
	for _, p := range plans {
		titles[p.Id] = p.Title
	}

	byPlan := make(map[int]*PlanSubscriptionStat, len(rows))
	for _, row := range rows {
		entry, ok := byPlan[row.PlanID]
		if !ok {
			entry = &PlanSubscriptionStat{PlanID: row.PlanID, Title: titles[row.PlanID]}
			byPlan[row.PlanID] = entry
		}
		entry.Total += row.Cnt
		if row.Status == "active" {
			entry.Active += row.Cnt
		}
	}
	for _, entry := range byPlan {
		stats.ByPlan = append(stats.ByPlan, *entry)
	}
	sort.Slice(stats.ByPlan, func(i, j int) bool { return stats.ByPlan[i].PlanID < stats.ByPlan[j].PlanID })
	return stats, nil
}
