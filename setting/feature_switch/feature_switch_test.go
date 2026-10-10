package feature_switch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

// resetState 把包状态与 common 的运行时覆盖清空，避免用例间串味。
func resetState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		setting.mu.Lock()
		setting.Values = map[string]string{}
		setting.mu.Unlock()
		common.ClearFeatureFlagOverrides()
	})
	setting.mu.Lock()
	setting.Values = map[string]string{}
	setting.mu.Unlock()
	common.ClearFeatureFlagOverrides()
}

func TestIsEnabledFallsBackToEnvDefaultWhenUnconfigured(t *testing.T) {
	resetState(t)

	// 未配置 + env 未设 → 注册表默认 false。
	assert.False(t, IsEnabled(common.FlagComplexityRouting))

	// env 设为 true，且仍无管理员配置 → 回退 env 默认。
	t.Setenv(common.FlagComplexityRouting, "true")
	assert.True(t, IsEnabled(common.FlagComplexityRouting),
		"未配置时必须回退 env 默认值")
}

func TestIsEnabledPrefersPersistedValueOverEnvDefault(t *testing.T) {
	resetState(t)

	// env 说开，管理员显式配置为关 → 以配置为准。
	t.Setenv(common.FlagRelayAuditEnabled, "true")
	require.NoError(t, Set(common.FlagRelayAuditEnabled, "false"))

	assert.False(t, IsEnabled(common.FlagRelayAuditEnabled),
		"管理员配置必须优先于 env 默认值")
	assert.False(t, common.FeatureFlagValue(common.FlagRelayAuditEnabled, true),
		"运行时快照也必须反映管理员配置")
}

func TestIsEnabledReturnsFalseForUnregisteredKey(t *testing.T) {
	resetState(t)

	assert.False(t, IsEnabled("NOT_A_REAL_SWITCH"),
		"未知开关必须 fail-closed 返回 false")
	assert.False(t, IsEnabled(""))
}

func TestIsEnabledIsFalseWhenDependencyUnsatisfied(t *testing.T) {
	resetState(t)

	// 注入一条带依赖的合成开关：启用它而不启用前置，必须判定为未生效。
	const (
		depKey  = "TEST_DEP_SWITCH_PREREQ"
		mainKey = "TEST_DEP_SWITCH_MAIN"
	)
	registry = append(registry,
		FeatureSwitch{Key: depKey, Kind: KindBool, Default: "false", Risk: RiskLow, AdminEditable: true},
		FeatureSwitch{Key: mainKey, Kind: KindBool, Default: "false", Risk: RiskLow, AdminEditable: true, DependsOn: []string{depKey}},
	)
	t.Cleanup(func() { registry = registry[:len(registry)-2] })

	// 前置未开：Set 应被拒绝。
	err := Set(mainKey, "true")
	var depErr *DependencyError
	require.ErrorAs(t, err, &depErr, "前置未满足时启用必须被拒绝")
	assert.Equal(t, depKey, depErr.Missing)
	assert.False(t, IsEnabled(mainKey))

	// 前置开启后，主开关才能启用并生效。
	require.NoError(t, Set(depKey, "true"))
	require.NoError(t, Set(mainKey, "true"))
	assert.True(t, IsEnabled(mainKey))

	// 关掉前置 → 主开关自动判定为未生效（依赖是实时的，不是一次性检查）。
	require.NoError(t, Set(depKey, "false"))
	assert.False(t, IsEnabled(mainKey),
		"前置关闭后，依赖它的开关必须立即视为未生效")
}

func TestIsEnabledSurvivesDependencyCycle(t *testing.T) {
	resetState(t)

	const (
		aKey = "TEST_CYCLE_A"
		bKey = "TEST_CYCLE_B"
	)
	registry = append(registry,
		FeatureSwitch{Key: aKey, Kind: KindBool, Default: "false", Risk: RiskLow, AdminEditable: true, DependsOn: []string{bKey}},
		FeatureSwitch{Key: bKey, Kind: KindBool, Default: "false", Risk: RiskLow, AdminEditable: true, DependsOn: []string{aKey}},
	)
	t.Cleanup(func() { registry = registry[:len(registry)-2] })

	assert.False(t, IsEnabled(aKey), "依赖成环必须判定为未生效且不得死循环")
}

func TestSetRejectsInvalidInput(t *testing.T) {
	resetState(t)

	var unknown *UnknownSwitchError
	require.ErrorAs(t, Set("NOPE", "true"), &unknown)

	var invalid *InvalidValueError
	require.ErrorAs(t, Set(common.FlagComplexityRouting, "maybe"), &invalid,
		"布尔开关只接受 true/false 词形")
	assert.Equal(t, []string{"true", "false"}, invalid.Allowed)

	// 枚举开关只接受注册的取值。
	require.ErrorAs(t, Set(common.FlagPolicyEngineMode, "aggressive"), &invalid)

	// 非法输入不得留下任何副作用。
	assert.False(t, IsEnabled(common.FlagComplexityRouting))
	_, configured := ConfiguredValue(common.FlagComplexityRouting)
	assert.False(t, configured, "被拒绝的写入不得留下配置残留")
}

func TestSetAcceptsEnumValues(t *testing.T) {
	resetState(t)

	for _, mode := range []string{"off", "shadow", "enforce"} {
		require.NoError(t, Set(common.FlagPolicyEngineMode, mode))
		assert.Equal(t, mode, common.FeatureFlagString(common.FlagPolicyEngineMode, "off"),
			"枚举开关必须原样保存 %s", mode)
	}

	// 大小写与空白应被归一化。
	require.NoError(t, Set(common.FlagPolicyEngineMode, "  SHADOW "))
	assert.Equal(t, "shadow", common.FeatureFlagString(common.FlagPolicyEngineMode, "off"))
}

func TestResetFallsBackToEnvDefault(t *testing.T) {
	resetState(t)
	t.Setenv(common.FlagMemoryInjectionEnabled, "true")

	require.NoError(t, Set(common.FlagMemoryInjectionEnabled, "false"))
	assert.False(t, IsEnabled(common.FlagMemoryInjectionEnabled))

	require.NoError(t, Reset(common.FlagMemoryInjectionEnabled))
	assert.True(t, IsEnabled(common.FlagMemoryInjectionEnabled),
		"Reset 后必须回退 env 默认值")

	_, configured := ConfiguredValue(common.FlagMemoryInjectionEnabled)
	assert.False(t, configured)
}

func TestPublishDropsUnknownAndMalformedKeys(t *testing.T) {
	resetState(t)

	setting.mu.Lock()
	setting.Values = map[string]string{
		common.FlagComplexityRouting: "true",  // 合法
		"INJECTED_UNKNOWN_KEY":       "true",  // 未注册 → 丢弃
		common.FlagPolicyEngineMode:  "bogus", // 非法枚举 → 丢弃
	}
	publishLocked()
	setting.mu.Unlock()

	assert.True(t, common.FeatureFlagValue(common.FlagComplexityRouting, false))
	assert.False(t, common.FeatureFlagValue("INJECTED_UNKNOWN_KEY", false),
		"未注册的 key 不得进入运行时快照（防持久化数据被污染后影响任意开关）")
	assert.Equal(t, "off", common.FeatureFlagString(common.FlagPolicyEngineMode, "off"),
		"非法枚举值不得进入运行时快照")
}

func TestSerializedValuesRoundTrips(t *testing.T) {
	resetState(t)

	require.NoError(t, Set(common.FlagRelayAuditEnabled, "true"))
	encoded, err := SerializedValues()
	require.NoError(t, err)

	// 走一遍 config 框架的写入路径（等价于启动时从库装载 / 管理端保存）。
	require.NoError(t, config.UpdateConfigFromMap(&setting, map[string]string{
		ValuesFieldKey: encoded,
	}))

	assert.True(t, IsEnabled(common.FlagRelayAuditEnabled),
		"经持久化格式往返后仍必须生效")
}

func TestSnapshotExposesRegistryMetadata(t *testing.T) {
	resetState(t)

	list := List()
	require.Len(t, list, len(registry), "快照必须覆盖注册表全部条目")

	byKey := map[string]Snapshot{}
	for _, s := range list {
		byKey[s.Key] = s
	}

	// 排序稳定（前端 diff 依赖）。
	for i := 1; i < len(list); i++ {
		assert.Less(t, list[i-1].Key, list[i].Key)
	}

	// 高风险开关必须带回滚提示（UI 的二次确认要显示它）。
	for _, key := range []string{common.FlagChannelKeyEncryption, common.FlagPasswordLoginEncryption} {
		s, ok := byKey[key]
		require.True(t, ok, "%s 必须在注册表内", key)
		assert.Equal(t, string(RiskHigh), s.Risk)
		assert.NotEmpty(t, s.RollbackHint, "%s 必须提供回滚提示", key)
	}

	// 已接入指标的开关必须声明指标名（UI 用它展示"打开后发生了什么"）。
	audit := byKey[common.FlagRelayAuditEnabled]
	assert.Contains(t, audit.MetricKeys, "relay_audit_findings_total")

	// 未配置时 Configured=false、Value 等于 env 默认。
	assert.False(t, audit.Configured)
	assert.Equal(t, "false", audit.Value)
}

func TestUnknownSwitchIsNeverEditableOrEnabled(t *testing.T) {
	resetState(t)

	err := Set("SOMETHING_ELSE", "true")
	assert.True(t, errors.As(err, new(*UnknownSwitchError)))
	assert.False(t, IsEnabled("SOMETHING_ELSE"))
}
