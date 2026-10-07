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
