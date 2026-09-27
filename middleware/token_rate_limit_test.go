package middleware

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAllowTokenQBS(t *testing.T) {
	// qbs=2：同一秒最多 2 次，之后拒。
	allow := true
	for i := 0; i < 2; i++ {
		if !allowTokenQBS(101, 2) {
			allow = false
		}
	}
	assert.True(t, allow, "前2次应允许")
	assert.False(t, allowTokenQBS(101, 2), "第3次应拒绝")

	// 空桶后，等待回填：qbs=2/s，过 1s 应回填 2 个令牌。
	time.Sleep(1100 * time.Millisecond)
	assert.True(t, allowTokenQBS(101, 2), "回填后应允许")
}

func TestTokenConcurrencyStore(t *testing.T) {
	assert.True(t, acquireTokenConcurrency(202, 2))
	assert.True(t, acquireTokenConcurrency(202, 2))
	assert.False(t, acquireTokenConcurrency(202, 2), "并发达到上限应拒绝")

	releaseTokenConcurrency(202)
	assert.True(t, acquireTokenConcurrency(202, 2), "释放后可再次获取")

	// 清场
	releaseTokenConcurrency(202)
	releaseTokenConcurrency(202)
}

func TestTokenRateLimitConfigParsing(t *testing.T) {
	// 通过 model 层的 GetRateLimitConfig 验证解析（中间件复用同一结构）。
	cfg := parseTokenRateLimitConfigFromString(`{"rpm":60,"qbs":5,"concurrency":2}`)
	assert.Equal(t, 60, cfg.RPM)
	assert.Equal(t, 5, cfg.QBS)
	assert.Equal(t, 2, cfg.Concurrency)

	empty := parseTokenRateLimitConfigFromString("")
	assert.Zero(t, empty.RPM)
	assert.Zero(t, empty.QBS)
	assert.Zero(t, empty.Concurrency)
}

func TestSubscriptionConcurrencyStore(t *testing.T) {
	assert.True(t, acquireSubscriptionConcurrency(301, 3))
	assert.True(t, acquireSubscriptionConcurrency(301, 3))
	assert.True(t, acquireSubscriptionConcurrency(301, 3))
	assert.False(t, acquireSubscriptionConcurrency(301, 3), "并发达到订阅上限应拒绝")

	releaseSubscriptionConcurrency(301)
	assert.True(t, acquireSubscriptionConcurrency(301, 3), "释放后可再次获取")

	releaseSubscriptionConcurrency(301)
	releaseSubscriptionConcurrency(301)
	releaseSubscriptionConcurrency(301)
}

func TestResolveSubscriptionTierWithoutSubscription(t *testing.T) {
	// 无有效用户或库不可用时安全放行（hasSub=false），不 panic。
	c, r, hasSub := resolveSubscriptionTier(0)
	assert.Zero(t, c)
	assert.Zero(t, r)
	assert.False(t, hasSub)

	c, r, hasSub = resolveSubscriptionTier(1)
	assert.Zero(t, c)
	assert.Zero(t, r)
	assert.False(t, hasSub)
}

func TestUserRateLimitConcurrencyStore(t *testing.T) {
	assert.True(t, acquireUserRateLimitConcurrency(401, 3))
	assert.True(t, acquireUserRateLimitConcurrency(401, 3))
	assert.True(t, acquireUserRateLimitConcurrency(401, 3))
	assert.False(t, acquireUserRateLimitConcurrency(401, 3), "达到基础并发上限应拒绝")

	releaseUserRateLimitConcurrency(401)
	assert.True(t, acquireUserRateLimitConcurrency(401, 3), "释放后可再次获取")

	releaseUserRateLimitConcurrency(401)
	releaseUserRateLimitConcurrency(401)
	releaseUserRateLimitConcurrency(401)
}

// TestResolveFromSubscriptionSummaries 锁定 §4.1.2 抽取函数（缓存命中路径）的档位语义：
// 覆盖优先、subs[0] 取最新、空/无 sub/套餐未命中 均安全返回。
func TestResolveFromSubscriptionSummaries(t *testing.T) {
	// 内存 DB：供套餐缓存未命中时回退查询（覆盖 plan-miss 分支，不 panic）。
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		model.InvalidateSubscriptionPlanCache(1)
	})

	// 套餐直接落内存 DB：GetSubscriptionPlanById 缓存未命中时回退 DB（无缓存 API 依赖）。
	plan := model.SubscriptionPlan{
		Title: "plan-a", ConcurrencyLimit: 3, RpmLimit: 50,
		DurationUnit: "month", DurationValue: 1, Enabled: true,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	planID := plan.Id

	newSub := func(rpmOv, conOv int) model.SubscriptionSummary {
		return model.SubscriptionSummary{
			Subscription: &model.UserSubscription{Id: 11, UserId: 63011, PlanId: planID, Status: "active", RpmOverride: rpmOv, ConcurrencyOverride: conOv},
		}
	}

	cases := []struct {
		name   string
		subs   []model.SubscriptionSummary
		expC   int
		expR   int
		expHas bool
	}{
		{"empty list", nil, 0, 0, false},
		{"nil subscription", []model.SubscriptionSummary{{Subscription: nil}}, 0, 0, false},
		{"plan limits no override", []model.SubscriptionSummary{newSub(0, 0)}, 3, 50, true},
		{"rpm override wins", []model.SubscriptionSummary{newSub(120, 0)}, 3, 120, true},
		{"concurrency override wins", []model.SubscriptionSummary{newSub(0, 9)}, 9, 50, true},
		{"both override", []model.SubscriptionSummary{newSub(120, 9)}, 9, 120, true},
		{"plan not found -> safe 0", []model.SubscriptionSummary{{Subscription: &model.UserSubscription{Id: 12, UserId: 63012, PlanId: 99999, Status: "active"}}}, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, r, has := resolveFromSubscriptionSummaries(tc.subs)
			assert.Equal(t, tc.expC, c)
			assert.Equal(t, tc.expR, r)
			assert.Equal(t, tc.expHas, has)
		})
	}
}
