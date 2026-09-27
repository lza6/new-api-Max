package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSiteSubscriptionStatsAggregates 覆盖 T15-A：站点订阅统计端点返回正确聚合，
// 且仅返回计数/标题，不含任何用户或订单明细。
func TestSiteSubscriptionStatsAggregates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := common.GetTimestamp()

	planA := model.SubscriptionPlan{Title: "Plan A", PriceAmount: 19.9, Currency: "USD", DurationUnit: "month", DurationValue: 1, Enabled: true, TotalAmount: 500000, QuotaResetPeriod: "month"}
	planB := model.SubscriptionPlan{Title: "Plan B", PriceAmount: 9.9, Currency: "USD", DurationUnit: "month", DurationValue: 1, Enabled: true, TotalAmount: 200000, QuotaResetPeriod: "month"}
	require.NoError(t, model.DB.Create(&planA).Error)
	require.NoError(t, model.DB.Create(&planB).Error)

	makeSub := func(userID, planID int, status string, endOffset, createdOffset int64) model.UserSubscription {
		sub := model.UserSubscription{
			UserId: userID, PlanId: planID, Status: status,
			StartTime: now - 86400, EndTime: now + endOffset,
		}
		require.NoError(t, model.DB.Create(&sub).Error)
		// BeforeCreate hook 会覆盖 CreatedAt，这里显式回写以测试"新近创建"口径。
		require.NoError(t, model.DB.Model(&sub).Update("created_at", now-createdOffset).Error)
		return sub
	}
	// Plan A: 2 active（其中 1 条 3 天内到期）+ 1 expired + 1 新近创建
	makeSub(1, planA.Id, "active", 86400*30, 86400*10)
	makeSub(2, planA.Id, "active", 86400*2, 86400*15)
	makeSub(3, planA.Id, "expired", -86400, 86400*90)
	makeSub(4, planA.Id, "active", 86400*20, 86400) // 新近创建（30 天内）且即将到期（7 天内）
	// Plan B: 1 active
	makeSub(5, planB.Id, "active", 86400*45, 86400*3)

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/stats/subscriptions", nil)
	GetSiteSubscriptionStats(c)

	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool                          `json:"success"`
		Data    model.SiteSubscriptionStats   `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	stats := payload.Data
	assert.Equal(t, 2, stats.TotalPlans)
	assert.Equal(t, int64(5), stats.TotalSubscriptions)
	assert.Equal(t, int64(4), stats.ActiveSubscriptions) // 4 active across both plans
	// active 且 7 天内到期：sub#2（2 天后）+ sub#4（20 天后？否）→ 仅 sub#2 与 sub#4 中 end<=weekOver
	// sub#4 end = now+20d > 7d，sub#2 end = now+2d <= 7d → 1 条；另有 sub#1 end=30d、sub#5 end=45d。
	assert.Equal(t, int64(1), stats.ExpiringSoon7d)
	assert.Equal(t, int64(4), stats.NewLast30d) // created within last 30d

	assert.Len(t, stats.ByPlan, 2)
	for _, entry := range stats.ByPlan {
		assert.NotEmpty(t, entry.Title)
		switch entry.PlanID {
		case planA.Id:
			assert.Equal(t, int64(4), entry.Total)
			assert.Equal(t, int64(3), entry.Active)
		case planB.Id:
			assert.Equal(t, int64(1), entry.Total)
			assert.Equal(t, int64(1), entry.Active)
		}
	}
	// 响应不泄露任何用户 ID / 订单 / 时间细节。
	assert.NotContains(t, response.Body.String(), "user_id")
	assert.NotContains(t, response.Body.String(), "start_time")
}
// TestAdminSetUserSubscriptionTierInvalidatesCache 锁定 v1.3.48 审计补位（GAP-A）：
// 管理端修改订阅档位覆盖（Rpm/ConcurrencyOverride）必须立即失效正向订阅缓存，
// 使新档位即时生效，而不是等 TTL 过期。
func TestAdminSetUserSubscriptionTierInvalidatesCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 造用户 + 套餐 + 订阅（创建路径会失效缓存，随后重新预置以模拟热路径已缓存）。
	password, err := common.HashAccountPassword("original-password-123")
	require.NoError(t, err)
	user := model.User{Username: "tier-cache-user", Email: "tier-cache@example.com", Password: password, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "tc1"}
	require.NoError(t, model.DB.Create(&user).Error)
	plan := model.SubscriptionPlan{Title: "tier-cache-plan", PriceAmount: 1, Currency: "USD", DurationUnit: "month", DurationValue: 1, Enabled: true, ConcurrencyLimit: 2, RpmLimit: 10, TotalAmount: 100000, QuotaResetPeriod: "month"}
	require.NoError(t, model.DB.Create(&plan).Error)
	sub, err := model.CreateUserSubscriptionFromPlanTx(model.DB, user.Id, &plan, "test")
	require.NoError(t, err)

	model.StoreCachedActiveSubscriptions(user.Id, []model.SubscriptionSummary{{Subscription: sub}})
	_, found := model.GetCachedActiveSubscriptions(user.Id)
	require.True(t, found, "热路径应已缓存该用户订阅")

	body := `{"rpm_override":100,"concurrency_override":5}`
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(sub.Id)}}
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/subscription/admin/user_subscriptions/"+strconv.Itoa(sub.Id)+"/tier", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminSetUserSubscriptionTier(c)

	require.Equal(t, http.StatusOK, response.Code)
	_, found = model.GetCachedActiveSubscriptions(user.Id)
	assert.False(t, found, "档位覆盖后正向订阅缓存应已失效")
	model.InvalidateActiveSubscriptionCache(user.Id)
}
