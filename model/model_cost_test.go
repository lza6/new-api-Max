package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelCostTest(t *testing.T) *gorm.DB {
	t.Helper()
	prevDB := DB
	prevRate := operation_setting.USDExchangeRate
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelCost{}))
	DB = db
	operation_setting.USDExchangeRate = 7.3
	modelCostCache.Range(func(k, _ any) bool { modelCostCache.Delete(k); return true })
	t.Cleanup(func() {
		DB = prevDB
		operation_setting.USDExchangeRate = prevRate
		modelCostCache.Range(func(k, _ any) bool { modelCostCache.Delete(k); return true })
	})
	return db
}

// 成本按「当时生效」的最新记录计算：调价只追加新行，旧行永远保留。
func TestModelCostTemporalSelection(t *testing.T) {
	db := setupModelCostTest(t)
	base := time.Unix(1_800_000_000, 0)

	_, err := CreateModelCost(ModelCostInput{
		ChannelId: 49, Model: "m", BillingMode: ModelCostModePerCall,
		CostPerCallCNY: 0.0007, EffectiveFrom: base.Unix(),
	}, 1)
	require.NoError(t, err)
	_, err = CreateModelCost(ModelCostInput{
		ChannelId: 49, Model: "m", BillingMode: ModelCostModePerCall,
		CostPerCallCNY: 0.0014, EffectiveFrom: base.Add(time.Hour).Unix(),
	}, 1)
	require.NoError(t, err)

	quotaBefore, ruleBefore, ok := RequestCostQuota(49, "m", 0, 0, base.Add(30*time.Minute))
	require.True(t, ok)
	quotaAfter, ruleAfter, ok := RequestCostQuota(49, "m", 0, 0, base.Add(2*time.Hour))
	require.True(t, ok)

	// 0.0007 元 / 7.3 * 500000 ≈ 47.95 → 48
	assert.Equal(t, int64(48), quotaBefore)
	// 0.0014 元 → 约 95.89 → 96
	assert.Equal(t, int64(96), quotaAfter)
	assert.NotEqual(t, ruleBefore, ruleAfter, "调价后应使用新规则")

	var count int64
	require.NoError(t, db.Model(&ModelCost{}).Count(&count).Error)
	assert.Equal(t, int64(2), count, "调价只能追加，不能覆盖旧行")
}

// 未配置模型级成本时回退到渠道级（model=""）。
func TestModelCostFallsBackToChannelDefault(t *testing.T) {
	setupModelCostTest(t)
	now := time.Now()
	_, err := CreateModelCost(ModelCostInput{
		ChannelId: 7, Model: "", BillingMode: ModelCostModePerCall,
		CostPerCallCNY: 0.002, EffectiveFrom: now.Add(-time.Minute).Unix(),
	}, 1)
	require.NoError(t, err)

	quota, _, ok := RequestCostQuota(7, "any-model", 0, 0, now)
	require.True(t, ok)
	// 0.002 / 7.3 * 500000 ≈ 136.99 → 137
	assert.Equal(t, int64(137), quota)
}

// 按 token 的成本：输入/输出分别按每百万 token 单价累加。
func TestModelCostPerToken(t *testing.T) {
	setupModelCostTest(t)
	now := time.Now()
	_, err := CreateModelCost(ModelCostInput{
		ChannelId: 9, Model: "t", BillingMode: ModelCostModePerToken,
		CostInputPerMillionCNY:  1, // 1 元 / 1M input
		CostOutputPerMillionCNY: 2, // 2 元 / 1M output
		EffectiveFrom:           now.Add(-time.Minute).Unix(),
	}, 1)
	require.NoError(t, err)

	quota, _, ok := RequestCostQuota(9, "t", 500_000, 250_000, now)
	require.True(t, ok)
	// (0.5 + 0.5) 元 / 7.3 * 500000 ≈ 68493.15 → 68493
	assert.Equal(t, int64(68493), quota)
}

func TestValidateModelCostInput(t *testing.T) {
	now := time.Now()
	bad := []ModelCostInput{
		{ChannelId: 0, Model: "m", BillingMode: ModelCostModePerCall, CostPerCallCNY: 1},
		{ChannelId: 1, Model: "m", BillingMode: "weird", CostPerCallCNY: 1},
		{ChannelId: 1, Model: "m", BillingMode: ModelCostModePerCall, CostPerCallCNY: -1},
		{ChannelId: 1, Model: "m", BillingMode: ModelCostModePerCall},
		{ChannelId: 1, Model: "m", BillingMode: ModelCostModePerToken},
		{ChannelId: 1, Model: "m", BillingMode: ModelCostModePerCall, CostPerCallCNY: 5000},
	}
	for i := range bad {
		in := bad[i]
		assert.Error(t, ValidateModelCostInput(&in, now), "case %d", i)
	}
	good := ModelCostInput{ChannelId: 1, Model: "m", BillingMode: ModelCostModePerCall, CostPerCallCNY: 0.0007}
	require.NoError(t, ValidateModelCostInput(&good, now))
}
