package billingexpr_test

import (
	"testing"

	"github.com/lza6/new-api-Max/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 计费维度选择：让同一个模型按分组或按渠道切换计费方式（按次 vs 按 token）。
// 对应需求：「单模型只要设计了一种计费就不能另一种计费方式了」。
const groupChannelExpr = `(group == "按次" || channel == 49)
  ? tier("per_request", fixed(0.02))
  : tier("per_token", p * 2 + c * 8)`

func TestGroupSelectsBillingMode(t *testing.T) {
	params := billingexpr.TokenParams{P: 1000, C: 500, Len: 1000}

	t.Run("group matched charges a fixed per-request price", func(t *testing.T) {
		cost, trace, err := billingexpr.RunExprWithRequest(groupChannelExpr, params,
			billingexpr.RequestInput{Group: "按次"})
		require.NoError(t, err)
		// fixed(0.02) 内部按 *1e6 缩放到配额域。
		assert.Equal(t, 20000.0, cost)
		assert.Equal(t, "per_request", trace.MatchedTier)
		assert.Equal(t, billingexpr.BillingUnitRequest, trace.BillingUnit)
	})

	t.Run("other group falls back to token pricing", func(t *testing.T) {
		cost, trace, err := billingexpr.RunExprWithRequest(groupChannelExpr, params,
			billingexpr.RequestInput{Group: "default"})
		require.NoError(t, err)
		assert.Equal(t, 1000*2.0+500*8.0, cost)
		assert.Equal(t, "per_token", trace.MatchedTier)
		assert.Equal(t, billingexpr.BillingUnitToken, trace.BillingUnit)
	})

	t.Run("empty group falls back to token pricing", func(t *testing.T) {
		cost, trace, err := billingexpr.RunExprWithRequest(groupChannelExpr, params,
			billingexpr.RequestInput{})
		require.NoError(t, err)
		assert.Equal(t, 1000*2.0+500*8.0, cost)
		assert.Equal(t, "per_token", trace.MatchedTier)
	})
}

func TestChannelSelectsBillingMode(t *testing.T) {
	params := billingexpr.TokenParams{P: 1000, C: 500, Len: 1000}

	t.Run("matched channel charges fixed price", func(t *testing.T) {
		cost, trace, err := billingexpr.RunExprWithRequest(groupChannelExpr, params,
			billingexpr.RequestInput{ChannelID: 49})
		require.NoError(t, err)
		assert.Equal(t, 20000.0, cost)
		assert.Equal(t, "per_request", trace.MatchedTier)
	})

	t.Run("other channel falls back to token pricing", func(t *testing.T) {
		cost, trace, err := billingexpr.RunExprWithRequest(groupChannelExpr, params,
			billingexpr.RequestInput{ChannelID: 7})
		require.NoError(t, err)
		assert.Equal(t, 1000*2.0+500*8.0, cost)
		assert.Equal(t, "per_token", trace.MatchedTier)
	})

	t.Run("channel zero means unknown and uses token pricing", func(t *testing.T) {
		cost, _, err := billingexpr.RunExprWithRequest(groupChannelExpr, params,
			billingexpr.RequestInput{})
		require.NoError(t, err)
		assert.Equal(t, 1000*2.0+500*8.0, cost)
	})
}

// group 与 channel 可组合使用，两者是独立维度。
func TestGroupAndChannelCombine(t *testing.T) {
	const expr = `group == "vip" && channel == 50
  ? tier("vip_ch50", fixed(0.05))
  : tier("base", p * 1)`
	params := billingexpr.TokenParams{P: 100, Len: 100}

	cost, trace, err := billingexpr.RunExprWithRequest(expr, params,
		billingexpr.RequestInput{Group: "vip", ChannelID: 50})
	require.NoError(t, err)
	assert.Equal(t, 50000.0, cost)
	assert.Equal(t, "vip_ch50", trace.MatchedTier)

	// 只满足一个条件 → 走 base。
	cost, trace, err = billingexpr.RunExprWithRequest(expr, params,
		billingexpr.RequestInput{Group: "vip", ChannelID: 51})
	require.NoError(t, err)
	assert.Equal(t, 100.0, cost)
	assert.Equal(t, "base", trace.MatchedTier)
}

// 存量表达式（不含 group/channel）必须行为完全不变——这是向后兼容的关键断言。
func TestExistingExpressionsUnaffectedByNewVariables(t *testing.T) {
	const expr = `len <= 32000 ? tier("short", fixed(0.01)) : tier("long", p * 2 + c * 8)`
	params := billingexpr.TokenParams{P: 1000, C: 500, Len: 1000}

	plain, tracePlain, err := billingexpr.RunExprWithRequest(expr, params, billingexpr.RequestInput{})
	require.NoError(t, err)

	withCtx, traceCtx, err := billingexpr.RunExprWithRequest(expr, params,
		billingexpr.RequestInput{Group: "按次", ChannelID: 49})
	require.NoError(t, err)

	assert.Equal(t, plain, withCtx, "存量表达式结果不得受 group/channel 影响")
	assert.Equal(t, tracePlain.MatchedTier, traceCtx.MatchedTier)
}

// 编译期必须认识 group/channel，否则保存表达式时会被拒。
func TestGroupChannelAreCompilable(t *testing.T) {
	for _, expr := range []string{
		`tier("a", group == "x" ? p * 1 : p * 2)`,
		`tier("a", channel == 49 ? p * 1 : p * 2)`,
		`tier("a", group + "x" == "yx" ? p * 1 : p * 2)`,
	} {
		_, err := billingexpr.CompileFromCache(expr)
		assert.NoError(t, err, "expression must compile: %s", expr)
	}
}
