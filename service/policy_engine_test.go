package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T10 策略引擎：off 模式零评估。
func TestPolicyEngineOffMode(t *testing.T) {
	off := PolicyModeOff
	SetPolicyMode(&off)
	defer SetPolicyMode(nil)
	require.False(t, PolicyEngineEnabled())
	d := Evaluate(PolicyInput{PromptChars: 999999, MaxPromptChar: 10})
	assert.Equal(t, PolicyAllow, d.Action)
}

// T10 策略引擎：shadow 模式评估但不拦截（Enforced=false）。
func TestPolicyEngineShadowMode(t *testing.T) {
	shadow := PolicyModeShadow
	SetPolicyMode(&shadow)
	defer SetPolicyMode(nil)
	ResetPolicyCountersForTest()

	d := Evaluate(PolicyInput{PromptChars: 100, MaxPromptChar: 10})
	assert.Equal(t, PolicyBlock, d.Action)
	assert.False(t, d.Enforced, "shadow mode must not enforce")
	assert.Equal(t, PolicyKindGuardrail, d.Kind)

	// 命中计数已记录。
	snap := PolicySnapshot()
	assert.Greater(t, snap["guardrail:block"], int64(0))
}

// T10 策略引擎：enforce 模式命中即拦截。
func TestPolicyEngineEnforceMode(t *testing.T) {
	enf := PolicyModeEnforce
	SetPolicyMode(&enf)
	defer SetPolicyMode(nil)

	d := Evaluate(PolicyInput{UserRemainingQuota: 100, RequestEstimate: 500})
	assert.Equal(t, PolicyBlock, d.Action)
	assert.True(t, d.Enforced, "enforce mode must enforce blocks")
	assert.Equal(t, PolicyKindBudget, d.Kind)
}

// T10 三方策略：护栏/预算/限流分别独立命中。
func TestPolicyEngineEachKind(t *testing.T) {
	enf := PolicyModeEnforce
	SetPolicyMode(&enf)
	defer SetPolicyMode(nil)

	tests := []struct {
		name string
		in   PolicyInput
		kind PolicyKind
	}{
		{"guardrail", PolicyInput{PromptChars: 100, MaxPromptChar: 50}, PolicyKindGuardrail},
		{"budget", PolicyInput{UserRemainingQuota: 10, RequestEstimate: 20}, PolicyKindBudget},
		{"rate_concurrency", PolicyInput{ConcurrencyLimit: 5, CurrentConcurrency: 6}, PolicyKindRate},
		{"rate_rpm", PolicyInput{RpmLimit: 100, CurrentRpm: 101}, PolicyKindRate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(tc.in)
			assert.Equal(t, PolicyBlock, d.Action)
			assert.Equal(t, tc.kind, d.Kind)
		})
	}
}

// T10 无命中 → allow。
func TestPolicyEngineAllow(t *testing.T) {
	enf := PolicyModeEnforce
	SetPolicyMode(&enf)
	defer SetPolicyMode(nil)
	d := Evaluate(PolicyInput{
		PromptChars: 10, MaxPromptChar: 100,
		UserRemainingQuota: 1000, RequestEstimate: 10,
		ConcurrencyLimit: 10, CurrentConcurrency: 1,
		RpmLimit: 100, CurrentRpm: 5,
	})
	assert.Equal(t, PolicyAllow, d.Action)
	assert.Empty(t, d.Kind)
}

// T-B4 统一观测：由其它路径（如 relay 早期敏感词拦截）执行的命中也要计入
// 策略中心的 guardrail 计数，且 RecordPolicyHit 绝不能改变判定结果。
func TestRecordPolicyHitFeedsUnifiedCounters(t *testing.T) {
	enf := PolicyModeEnforce
	SetPolicyMode(&enf)
	defer SetPolicyMode(nil)
	ResetPolicyCountersForTest()

	before := PolicySnapshot()
	RecordPolicyHit(PolicyKindGuardrail, PolicyBlock)
	after := PolicySnapshot()

	key := string(PolicyKindGuardrail) + ":" + string(PolicyBlock)
	assert.Equal(t, before[key]+1, after[key], "external guardrail hit must be counted")

	// 只计数，不产生任何判定影响。
	assert.Equal(t, PolicyAllow, Evaluate(PolicyInput{
		PromptChars: 10, MaxPromptChar: 100,
		UserRemainingQuota: 1000, RequestEstimate: 10,
	}).Action)
}

// T-B5 模型级单请求上限：超限拦截、未超放行、未配置（0）放行。
func TestPolicyEngineModelRequestLimit(t *testing.T) {
	enf := PolicyModeEnforce
	SetPolicyMode(&enf)
	defer SetPolicyMode(nil)

	base := PolicyInput{UserRemainingQuota: 1_000_000, RequestEstimate: 5_000, Model: "gpt-test"}

	// 未配置上限（0）→ 放行。
	assert.Equal(t, PolicyAllow, Evaluate(base).Action)

	// 上限高于预估 → 放行。
	ok := base
	ok.ModelRequestLimit = 10_000
	assert.Equal(t, PolicyAllow, Evaluate(ok).Action)

	// 低于预估 → 拦截，且规则名可解释。
	over := base
	over.ModelRequestLimit = 4_000
	d := Evaluate(over)
	assert.Equal(t, PolicyBlock, d.Action)
	assert.True(t, d.Enforced)
	assert.Equal(t, PolicyKindBudget, d.Kind)
	assert.Equal(t, "model_request_limit", d.Rule)
}
