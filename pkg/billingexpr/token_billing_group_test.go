package billingexpr_test

import (
	"testing"

	"github.com/lza6/new-api-Max/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 专用「token 计费」分组：按输入 token 用量阶梯计价（每次调用固定价）。
//   < 200K  → $0.001
//   ≥ 200K  → $0.0015
//   ≥ 500K  → $0.002
// 其他分组（如按次调用）保持原价 $0.002 不变。
const tokenBillingGroup = "token计费"

const tokenBillingExpr = `group == "token计费"
  ? (len < 200000
      ? tier("tok_lt200k", fixed(0.001))
      : (len < 500000
          ? tier("tok_200k_500k", fixed(0.0015))
          : tier("tok_ge500k", fixed(0.002))))
  : tier("per_request", fixed(0.002))`

// fixed() 内部按 *1e6 缩放到配额域；QuotaPerUnit=500000 时，
// $0.001 → 500 quota，$0.0015 → 750，$0.002 → 1000。
func TestTokenBillingGroupTiers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		len   float64
		cost  float64
		tier  string
		token string
	}{
		{"below 200K charges base rate", 199999, 1000, "tok_lt200k", "token计费"},
		{"exactly 200K moves up a tier", 200000, 1500, "tok_200k_500k", "token计费"},
		{"between 200K and 500K", 499999, 1500, "tok_200k_500k", "token计费"},
		{"exactly 500K moves to top tier", 500000, 2000, "tok_ge500k", "token计费"},
		{"well beyond 500K", 1200000, 2000, "tok_ge500k", "token计费"},
		{"zero context still charges base", 0, 1000, "tok_lt200k", "token计费"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, trace, err := billingexpr.RunExprWithRequest(tokenBillingExpr,
				billingexpr.TokenParams{Len: tc.len},
				billingexpr.RequestInput{Group: tc.token})
			require.NoError(t, err)
			assert.Equal(t, tc.cost, cost, "tier %s", tc.tier)
			assert.Equal(t, tc.tier, trace.MatchedTier)
			assert.Equal(t, billingexpr.BillingUnitRequest, trace.BillingUnit)
		})
	}
}

// 其他分组必须保持原有按次价（$0.002），完全不受本方案影响。
func TestOtherGroupsKeepFlatPerRequestPrice(t *testing.T) {
	for _, group := range []string{"default", "你最爱的破jia", "优质渠道", ""} {
		t.Run("group="+group, func(t *testing.T) {
			// 即使上下文很大，非 token 计费分组也走固定 0.002。
			cost, trace, err := billingexpr.RunExprWithRequest(tokenBillingExpr,
				billingexpr.TokenParams{Len: 800000},
				billingexpr.RequestInput{Group: group})
			require.NoError(t, err)
			assert.Equal(t, 2000.0, cost)
			assert.Equal(t, "per_request", trace.MatchedTier)
		})
	}
}

// 阶梯边界必须单调不减：token 越多单价不会下降。
func TestTokenBillingTiersAreMonotonic(t *testing.T) {
	prev := -1.0
	for _, l := range []float64{0, 100000, 199999, 200000, 300000, 499999, 500000, 999999} {
		cost, _, err := billingexpr.RunExprWithRequest(tokenBillingExpr,
			billingexpr.TokenParams{Len: l},
			billingexpr.RequestInput{Group: tokenBillingGroup})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, cost, prev, "cost must not decrease at len=%v", l)
		prev = cost
	}
}

// 关键安全等价性（生产事故的防线）：除「token计费」分组外，表达式 else 分支
// 必须与现状（ratio 模式的按次价 $0.002）**逐分组完全等值**，否则切换瞬间会
// 改变其他分组的实际扣费 —— 这正是上一轮生产事故的成因。
func TestElseBranchEqualsLegacyPerRequestPrice(t *testing.T) {
	const quotaPerUnit = 500000.0
	const legacyPriceUSD = 0.002

	// tiered 路径：quota = exprOut/1e6 * QuotaPerUnit * groupRatio；
	// fixed(x) 内部 = x*1e6，故等价于 x * QuotaPerUnit * groupRatio。
	for _, gr := range []float64{1.0, 1.5} {
		for _, group := range []string{"default", "你最爱的破jia", "优质渠道", ""} {
			cost, _, err := billingexpr.RunExprWithRequest(tokenBillingExpr,
				billingexpr.TokenParams{Len: 900000},
				billingexpr.RequestInput{Group: group})
			require.NoError(t, err)
			tieredQuota := cost / 1_000_000 * quotaPerUnit * gr
			legacyQuota := legacyPriceUSD * quotaPerUnit * gr
			assert.InDelta(t, legacyQuota, tieredQuota, 1e-6,
				"group=%q ratio=%v：else 分支必须与现状等值", group, gr)
		}
	}
}
