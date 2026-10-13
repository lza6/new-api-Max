package service

import (
	"testing"

	"github.com/lza6/new-api-Max/setting/routing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G4 阶段 A（shadow）用例集。
//
// 指南 §6.2.1 的硬约束逐条锁死：
//   - 路由结果不得越过用户可用分组/模型白名单；
//   - 无可用候选时回退原模型（不得变成「无可用渠道」）；
//   - 路由不得改变计费口径（本阶段恒不改，见 ApplyEnforceGuard 的恒等性）。
//
// 其中「越权候选被拒」这条要求**反向验证**：把白名单校验去掉后该用例必须失败。

func withRoutingSetting(t *testing.T, mode string, tierModels map[string][]string) {
	t.Helper()
	previous := routing_setting.GetRoutingSetting()
	t.Cleanup(func() {
		routing_setting.UpdateRoutingSetting(func(s *routing_setting.RoutingSetting) {
			s.Mode = previous.Mode
			s.TierModels = previous.TierModels
		})
	})
	routing_setting.UpdateRoutingSetting(func(s *routing_setting.RoutingSetting) {
		s.Mode = mode
		s.TierModels = tierModels
	})
}

func TestRoutingModeNormalizesUnknownToOff(t *testing.T) {
	withRoutingSetting(t, "ENFORCE-something-typo", nil)
	assert.Equal(t, RoutingModeOff, RoutingMode(),
		"配置写错时必须回退 off，绝不能意外进入 enforce")
	assert.False(t, RoutingEnabled())
}

func TestRoutingShadowNoConfiguredCandidatesIsNoDecision(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{})

	callCount := 0
	decision := ResolveRoutingShadow(ComplexityTierComplex, "gpt-x", func(string) bool {
		callCount++
		return true
	})

	assert.Empty(t, decision.WouldUse, "管理员没配置候选时不得产生任何路由决策")
	assert.Equal(t, RoutingRejectNoConfig, decision.Reason)
	assert.Zero(t, callCount, "无候选时不应白跑可用性校验")
}

func TestRoutingShadowPicksFirstAvailableCandidate(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{
		string(ComplexityTierSimple): {"cheap-model", "medium-model"},
	})

	available := map[string]bool{"cheap-model": true, "medium-model": true}
	decision := ResolveRoutingShadow(ComplexityTierSimple, "expensive-model", func(m string) bool {
		return available[m]
	})

	require.Equal(t, "cheap-model", decision.WouldUse, "应按配置顺序取首个可用候选")
	assert.Equal(t, []string{"cheap-model", "medium-model"}, decision.Resolved)
	assert.Equal(t, "shadow_only", decision.Reason)
}

// 反向验证锚点：**越权候选必须被拒**。去掉 ResolveRoutingShadow 里的
// isModelAvailable 判断，本用例会立即失败（候选会被放进 Resolved）。
func TestRoutingShadowRejectsCandidateOutsideUserWhitelist(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{
		string(ComplexityTierSimple): {"not-in-my-group", "allowed-model"},
	})

	// 只有 allowed-model 对该用户可用。
	decision := ResolveRoutingShadow(ComplexityTierSimple, "expensive-model", func(m string) bool {
		return m == "allowed-model"
	})

	assert.Equal(t, []string{"allowed-model"}, decision.Resolved,
		"越权候选不得进入 Resolved")
	assert.Equal(t, "allowed-model", decision.WouldUse)
	require.Len(t, decision.Rejected, 1)
	assert.Equal(t, "not-in-my-group", decision.Rejected[0].Model)
	assert.Equal(t, RoutingRejectNotInUserGroup, decision.Rejected[0].Reason)
}

func TestRoutingShadowSkipsCandidateEqualToRequestedModel(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{
		string(ComplexityTierMedium): {"same-model", "other-model"},
	})

	decision := ResolveRoutingShadow(ComplexityTierMedium, "same-model", func(string) bool { return true })

	assert.Equal(t, "other-model", decision.WouldUse, "候选等于原模型时不算改路由，应继续取下一个")
	require.Len(t, decision.Rejected, 1)
	assert.Equal(t, RoutingRejectSameAsRequested, decision.Rejected[0].Reason)
}

// 指南要求：无可用候选 → **回退原模型**并记录，不得变成「无可用渠道」。
func TestRoutingShadowFallsBackWhenNoCandidateAvailable(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{
		string(ComplexityTierComplex): {"unavailable-a", "unavailable-b"},
	})

	decision := ResolveRoutingShadow(ComplexityTierComplex, "original-model", func(string) bool { return false })

	assert.Empty(t, decision.WouldUse, "无可用候选时必须回退（空决策 = 保持原模型）")
	assert.Equal(t, RoutingRejectNotInUserGroup, decision.Reason)
	assert.Len(t, decision.Rejected, 2, "两个候选都应被记录为剔除，便于校准配置")
}

// 阶段 A 的定义就是「不改路由」：无论配置了什么，ApplyEnforceGuard 在
// non-enforce 模式下必须原样返回请求的模型。
func TestApplyEnforceGuardDoesNotChangeModelOutsideEnforce(t *testing.T) {
	for _, mode := range []string{RoutingModeOff, RoutingModeShadow} {
		withRoutingSetting(t, mode, map[string][]string{
			string(ComplexityTierSimple): {"cheap-model"},
		})
		decision := ResolveRoutingShadow(ComplexityTierSimple, "requested-model", func(string) bool { return true })
		require.Equal(t, "cheap-model", decision.WouldUse, "%s 模式下影子决策应算出候选", mode)

		got := ApplyEnforceGuard(decision, "requested-model")
		assert.Equal(t, "requested-model", got,
			"%s 模式下绝不能改变实际模型（阶段 A 的零行为变化）", mode)
	}
}

func TestApplyEnforceGuardEnforceUsesResolvedCandidate(t *testing.T) {
	withRoutingSetting(t, RoutingModeEnforce, map[string][]string{
		string(ComplexityTierSimple): {"cheap-model"},
	})
	decision := ResolveRoutingShadow(ComplexityTierSimple, "requested-model", func(string) bool { return true })

	assert.Equal(t, "cheap-model", ApplyEnforceGuard(decision, "requested-model"))
}

// enforce 模式下若候精选不出来（越权/无渠道），必须回退原模型——
// 不得把请求变成「无可用渠道」。
func TestApplyEnforceGuardEnforceFallsBackOnEmptyDecision(t *testing.T) {
	withRoutingSetting(t, RoutingModeEnforce, map[string][]string{
		string(ComplexityTierSimple): {"not-allowed"},
	})
	decision := ResolveRoutingShadow(ComplexityTierSimple, "requested-model", func(string) bool { return false })
	require.Empty(t, decision.WouldUse)

	assert.Equal(t, "requested-model", ApplyEnforceGuard(decision, "requested-model"),
		"enforce 下无可用候选必须回退原模型")
}

// 决策快照必须是深拷贝：热更新就地写入不能影响已发出的快照，否则
// map 并发读写（Go 内存模型下未定义行为，可 fatal）。
func TestRoutingSettingSnapshotIsDetachedFromLiveCopy(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{
		string(ComplexityTierSimple): {"a-model"},
	})

	snap := routing_setting.GetRoutingSetting()
	require.Equal(t, []string{"a-model"}, snap.TierModels[string(ComplexityTierSimple)])

	// 热更新覆盖配置。
	routing_setting.UpdateRoutingSetting(func(s *routing_setting.RoutingSetting) {
		s.TierModels[string(ComplexityTierSimple)] = []string{"b-model", "c-model"}
	})

	assert.Equal(t, []string{"a-model"}, snap.TierModels[string(ComplexityTierSimple)],
		"旧快照不得被后续写入改动（否则构成并发读写）")
	assert.Equal(t, []string{"b-model", "c-model"},
		routing_setting.GetRoutingSetting().TierModels[string(ComplexityTierSimple)])
}

func TestRoutingShadowRejectionReasonsAreDedupedAndSorted(t *testing.T) {
	withRoutingSetting(t, RoutingModeShadow, map[string][]string{
		string(ComplexityTierComplex): {"same", "gone", "same2", "gone2"},
	})
	decision := ResolveRoutingShadow(ComplexityTierComplex, "same", func(m string) bool {
		// same2 可用；gone/gone2 不可用。
		return m == "same2"
	})
	// same → same_as_requested；gone → not_available；same2 → 保留；gone2 → not_available
	reasons := RoutingShadowRejectionReasons(decision)
	assert.Equal(t, []string{RoutingRejectNotInUserGroup, RoutingRejectSameAsRequested}, reasons,
		"原因应去重且稳定排序（供日志聚合）")
}
