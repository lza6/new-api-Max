package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedSubscriptionAccessDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, connErr := db.DB()
	require.NoError(t, connErr)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	return db
}

func TestCheckSubscriptionModelAccess(t *testing.T) {
	// fail-open：无订阅/非法输入/DB 不可用一律放行。
	allowed, _, err := CheckSubscriptionModelAccess(0, "deepseek-v4-flash")
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, _, err = CheckSubscriptionModelAccess(1, "")
	require.NoError(t, err)
	require.True(t, allowed)

	previousDB := model.DB
	model.DB = nil
	allowed, _, err = CheckSubscriptionModelAccess(1, "deepseek-v4-flash")
	require.NoError(t, err)
	require.True(t, allowed)
	model.DB = previousDB
}

func TestCheckSubscriptionModelAccessMatrix(t *testing.T) {
	db := seedSubscriptionAccessDB(t)
	now := common.GetTimestamp()
	plan := &model.SubscriptionPlan{
		Title:            "天卡无限",
		Models:           `["deepseek-v4-flash","gpt-4o-mini"]`,
		DurationUnit:     model.SubscriptionDurationDay,
		DurationValue:    1,
		PriceAmount:      2,
		TotalAmount:      0,
		ConcurrencyLimit: 3,
		RpmLimit:         150,
	}
	require.NoError(t, db.Create(plan).Error)
	sub := &model.UserSubscription{
		UserId:    61001,
		PlanId:    plan.Id,
		Status:    "active",
		StartTime: now,
		EndTime:   now + 86400,
	}
	require.NoError(t, db.Create(sub).Error)

	allowed, reason, err := CheckSubscriptionModelAccess(sub.UserId, "deepseek-v4-flash")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Empty(t, reason)

	allowed, reason, err = CheckSubscriptionModelAccess(sub.UserId, "claude-3-5-sonnet")
	require.NoError(t, err)
	require.False(t, allowed)
	require.Contains(t, reason, "不在当前订阅套餐")
	require.Contains(t, reason, "Tf00798")

	// 套餐 Models 为空 = 不限。
	openPlan := &model.SubscriptionPlan{Title: "开放套餐", Models: "", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, PriceAmount: 1}
	require.NoError(t, db.Create(openPlan).Error)
	openSub := &model.UserSubscription{UserId: 61002, PlanId: openPlan.Id, Status: "active", StartTime: now, EndTime: now + 86400}
	require.NoError(t, db.Create(openSub).Error)
	allowed, reason, err = CheckSubscriptionModelAccess(openSub.UserId, "any-model")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Empty(t, reason)
}

func TestNoSubscriptionNegativeCacheAvoidsDB(t *testing.T) {
	seedSubscriptionAccessDB(t)
	// 真实 DB：用户 62001 无订阅 → 首次解析应缓存"无订阅"。
	allowed, _, err := CheckSubscriptionModelAccess(62001, "deepseek-v4-flash")
	require.NoError(t, err)
	require.True(t, allowed)
	require.True(t, HasCachedNoSubscription(62001))

	// 置 DB 为 nil 后二次调用仍放行（缓存命中，未触 DB）——证明热路径免 DB 读。
	previous := model.DB
	model.DB = nil
	allowed, _, err = CheckSubscriptionModelAccess(62001, "deepseek-v4-flash")
	require.NoError(t, err)
	require.True(t, allowed)
	model.DB = previous

	// 订阅用户不缓存（保持实时正确）。
	require.False(t, HasCachedNoSubscription(61001))
}

func TestCheckSubscriptionGroupAccess(t *testing.T) {
	s := relay_setting.GetRelaySetting()
	prevGroups := s.SubscriptionRequiredGroups
	defer func() { s.SubscriptionRequiredGroups = prevGroups }()
	s.SubscriptionRequiredGroups = []string{"subscriber"}

	// 未标记分组放行。
	allowed, reason, err := CheckSubscriptionGroupAccess(1, "default")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Empty(t, reason)

	// DB 不可用 fail-open。
	previousDB := model.DB
	model.DB = nil
	allowed, _, err = CheckSubscriptionGroupAccess(1, "subscriber")
	require.NoError(t, err)
	require.True(t, allowed)
	model.DB = previousDB

	db := seedSubscriptionAccessDB(t)
	now := common.GetTimestamp()
	// 有订阅 → 放行。
	plan := &model.SubscriptionPlan{Title: "门禁卡", Models: "", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, PriceAmount: 1}
	require.NoError(t, db.Create(plan).Error)
	sub := &model.UserSubscription{UserId: 63001, PlanId: plan.Id, Status: "active", StartTime: now, EndTime: now + 86400}
	require.NoError(t, db.Create(sub).Error)
	allowed, reason, err = CheckSubscriptionGroupAccess(63001, "subscriber")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Empty(t, reason)

	// 未订阅 + 需订阅分组 → 拒绝且文案可读。
	allowed, reason, err = CheckSubscriptionGroupAccess(63002, "subscriber")
	require.NoError(t, err)
	require.False(t, allowed)
	require.Contains(t, reason, "需持有订阅")
	require.Contains(t, reason, "Tf00798")
}
