package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestAdminDeleteSubscriptionPlanDeletesPlanWithActiveSubscription
// 产品决策（2026-10-01）：套餐被活跃订阅引用时**仍可删除**；订阅保留原
// end_time（不延长不剥夺），后续计费/限流/访问判定由配置快照兜底。
func TestAdminDeleteSubscriptionPlanDeletesPlanWithActiveSubscription(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id: 9701, Title: "Pro", PriceAmount: 10, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		TotalAmount: 1000, UpgradeGroup: "vip", ConcurrencyLimit: 3, RpmLimit: 150,
	}
	require.NoError(t, DB.Create(plan).Error)
	now := GetDBTimestamp()
	originalEnd := now + 30*24*3600
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9702, UserId: 501, PlanId: plan.Id, AmountTotal: 1000,
		StartTime: now - 3600, EndTime: originalEnd, Status: "active",
		UpgradeGroup: "vip", PrevUserGroup: "default",
	}).Error)

	affected, err := AdminDeleteSubscriptionPlan(plan.Id)
	require.NoError(t, err, "有订阅的套餐必须能删除（永远不能因历史记录被卡）")
	assert.Equal(t, []int{501}, affected)

	// 套餐行已删除。
	var count int64
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Count(&count).Error)
	assert.Zero(t, count)

	// 订阅保留原样：end_time 不变（既不延长也不剥夺），status 不变。
	var sub UserSubscription
	require.NoError(t, DB.Where("id = ?", 9702).First(&sub).Error)
	assert.Equal(t, originalEnd, sub.EndTime, "订阅必须按原到期时间自然结束（不延长不剥夺）")
	assert.Equal(t, "active", sub.Status)

	// 计费/访问判定仍能拿到快照配置（plan 行已删 → 回落 subscription_plan_grant）。
	plan2, err := GetPlanForSubscription(&sub)
	require.NoError(t, err)
	require.NotNil(t, plan2)
	assert.Equal(t, "Pro", plan2.Title, "快照必须保留套餐标题，删套餐后持续可用")
	assert.Equal(t, int64(1000), plan2.TotalAmount)
	assert.Equal(t, 150, plan2.RpmLimit)

	info, infoErr := GetSubscriptionPlanInfoByUserSubscriptionId(9702)
	require.NoError(t, infoErr)
	require.NotNil(t, info)
	assert.Equal(t, "Pro", info.PlanTitle)
}

// TestAdminDeleteSubscriptionPlanDeletesPlanWithHistoricalRecordOnly
// 套餐仅剩历史（过期/作废）订阅时删除：直接删套餐，订阅原样保留（不会变永久）。
func TestAdminDeleteSubscriptionPlanDeletesPlanWithHistoricalRecordOnly(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id: 9703, Title: "Old", PriceAmount: 5, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 500,
	}
	require.NoError(t, DB.Create(plan).Error)
	now := GetDBTimestamp()
	expiredEnd := now - 3600
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9704, UserId: 502, PlanId: plan.Id, AmountTotal: 500,
		StartTime: now - 7200, EndTime: expiredEnd, Status: "expired",
	}).Error)

	affected, err := AdminDeleteSubscriptionPlan(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, []int{502}, affected)

	var count int64
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Count(&count).Error)
	assert.Zero(t, count)

	// 历史订阅的 end_time 不被改动。
	var sub UserSubscription
	require.NoError(t, DB.Where("id = ?", 9704).First(&sub).Error)
	assert.Equal(t, expiredEnd, sub.EndTime)
}

func TestAdminDeleteSubscriptionPlanRemovesUnreferencedPlan(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id: 9801, Title: "Disposable", PriceAmount: 5, DurationUnit: SubscriptionDurationDay, DurationValue: 1, TotalAmount: 500,
	}
	require.NoError(t, DB.Create(plan).Error)

	affected, err := AdminDeleteSubscriptionPlan(plan.Id)
	require.NoError(t, err)
	assert.Empty(t, affected)

	var count int64
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Count(&count).Error)
	assert.Zero(t, count)

	// Historical orders keep their plan_id as a durable record and are untouched.
	order := &SubscriptionOrder{
		Id: 9802, UserId: 601, PlanId: plan.Id, Money: 5,
		TradeNo: "del-plan-historical", PaymentMethod: PaymentMethodBalance, Status: "success",
	}
	require.NoError(t, order.Insert())
	assert.NotNil(t, GetSubscriptionOrderByTradeNo("del-plan-historical"))
}

func TestAdminDeleteSubscriptionPlanMissingPlanReturnsNotFound(t *testing.T) {
	truncateTables(t)

	_, err := AdminDeleteSubscriptionPlan(999999)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not found") || err == gorm.ErrRecordNotFound)
}

func TestAdminDeleteSubscriptionPlanRejectsInvalidId(t *testing.T) {
	_, err := AdminDeleteSubscriptionPlan(0)
	require.Error(t, err)
	_, err = AdminDeleteSubscriptionPlan(-3)
	require.Error(t, err)
}