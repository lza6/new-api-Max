package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdminDeleteSubscriptionPlanRefusedWhileSubscriptionsReferenceIt(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:            9701,
		Title:         "Kept",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	now := GetDBTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9702, UserId: 501, PlanId: plan.Id, AmountTotal: 1000,
		StartTime: now, EndTime: now + 3600, Status: "cancelled",
	}).Error)

	err := AdminDeleteSubscriptionPlan(plan.Id)
	require.Error(t, err)
	// Even a cancelled subscription blocks deletion: no silent downgrade, no orphan.
	assert.Contains(t, err.Error(), "订阅记录")

	var count int64
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAdminDeleteSubscriptionPlanRemovesUnreferencedPlan(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:            9801,
		Title:         "Disposable",
		PriceAmount:   5,
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 1,
		TotalAmount:   500,
	}
	require.NoError(t, DB.Create(plan).Error)

	require.NoError(t, AdminDeleteSubscriptionPlan(plan.Id))

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

	err := AdminDeleteSubscriptionPlan(999999)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not found") || err == gorm.ErrRecordNotFound)
}

func TestAdminDeleteSubscriptionPlanRejectsInvalidId(t *testing.T) {
	require.Error(t, AdminDeleteSubscriptionPlan(0))
	require.Error(t, AdminDeleteSubscriptionPlan(-3))
}
