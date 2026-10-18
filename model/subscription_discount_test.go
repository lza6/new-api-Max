package model

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
)

func TestEffectiveSubscriptionPrice(t *testing.T) {
	plan := &SubscriptionPlan{PriceAmount: 100, FirstPurchaseDiscountPercent: 20}

	assert.Equal(t, 100.0, EffectiveSubscriptionPrice(plan, true, true), "有历史订阅不优惠")
	assert.Equal(t, 100.0, EffectiveSubscriptionPrice(plan, false, false), "开关关闭不优惠")
	assert.Equal(t, 80.0, EffectiveSubscriptionPrice(plan, false, true), "首购 20% 优惠")
	assert.Equal(t, 100.0, EffectiveSubscriptionPrice(&SubscriptionPlan{PriceAmount: 100}, false, true), "未配置折扣")

	plan.FirstPurchaseDiscountPercent = 150
	assert.Equal(t, 0.0, EffectiveSubscriptionPrice(plan, false, true), "折扣上限夹取到 100%")
}

func TestBuildSubscriptionSummarySavings(t *testing.T) {
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 1000
	t.Cleanup(func() { common.QuotaPerUnit = previous })

	// 用了 500000 quota = 500 货币单位；本周期实付 100；应省 400。
	sub := UserSubscription{
		AmountTotal: 1_000_000,
		AmountUsed:  500_000,
		PeriodPrice: 100,
		Currency:    "CNY",
	}
	summary := BuildSubscriptionSummary(sub)
	assert.Equal(t, int64(500_000), summary.ConsumedQuota)
	assert.Equal(t, int64(500_000), summary.RemainingQuota)
	assert.Equal(t, 500.0, summary.PayAsYouGoValue)
	assert.Equal(t, 400.0, summary.SavedAmount)
	assert.Equal(t, "CNY", summary.Currency)

	// 未用满套餐价时不得显示负数节省。
	sub.AmountUsed = 10_000
	summary = BuildSubscriptionSummary(sub)
	assert.Equal(t, 0.0, summary.SavedAmount)
}
