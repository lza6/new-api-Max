package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Batch-9 / G3 §5.2.2：工具抽屉的**请求级 opt-in**契约。
//
// 背景：全局开关一旦打开会影响所有客户端，而依赖完整 tool schema 的调用方可能被破坏。
// 把决定权下放到请求头 `X-NewAPI-Tool-Drawer` 后，「保守客户端不受影响」与
// 「想省 token 的客户端自助开启」可以同时成立。
//
// 优先级：**请求头 > 全局开关**；未声明请求头时沿用全局开关（缺省与历史行为一致）。

func setToolDrawerGlobal(t *testing.T, enabled bool) {
	t.Helper()
	SetToolDrawerEnabled(&enabled)
	t.Cleanup(func() { SetToolDrawerEnabled(nil) })
}

func requestWithToolDrawerHeader(value string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if value != "" {
		c.Request.Header.Set(ToolDrawerHeader, value)
	}
	return c
}

func TestResolveToolDrawerMode_HeaderOverridesGlobalSwitch(t *testing.T) {
	// 全局开，但请求头要求关 → 必须关（保守客户端不被全局开关伤害）
	setToolDrawerGlobal(t, true)
	assert.Equal(t, ToolDrawerModeOff, ResolveToolDrawerMode(requestWithToolDrawerHeader("off")),
		"请求头 off 必须覆盖全局开")

	// 全局关，但请求头要求去重 → 必须去重（自助开启）
	SetToolDrawerEnabled(nil)
	off := false
	SetToolDrawerEnabled(&off)
	assert.Equal(t, ToolDrawerModeDedupe, ResolveToolDrawerMode(requestWithToolDrawerHeader("dedupe")),
		"请求头 dedupe 必须覆盖全局关")
}

func TestResolveToolDrawerMode_FollowsGlobalSwitchWhenHeaderAbsent(t *testing.T) {
	setToolDrawerGlobal(t, true)
	assert.Equal(t, ToolDrawerModeDedupe, ResolveToolDrawerMode(requestWithToolDrawerHeader("")),
		"未声明请求头时沿用全局开关")

	off := false
	SetToolDrawerEnabled(&off)
	assert.Equal(t, ToolDrawerModeOff, ResolveToolDrawerMode(requestWithToolDrawerHeader("")),
		"全局关 + 未声明请求头 = 完全不动（与历史行为逐字节一致）")
}

func TestResolveToolDrawerMode_IsCaseAndSpaceInsensitive(t *testing.T) {
	off := false
	SetToolDrawerEnabled(&off)
	for _, raw := range []string{"OFF", " off ", "Off"} {
		assert.Equal(t, ToolDrawerModeOff, ResolveToolDrawerMode(requestWithToolDrawerHeader(raw)),
			"取值应大小写与空白无关：%q", raw)
	}
	for _, raw := range []string{"DEDUPE", " dedupe "} {
		assert.Equal(t, ToolDrawerModeDedupe, ResolveToolDrawerMode(requestWithToolDrawerHeader(raw)),
			"取值应大小写与空白无关：%q", raw)
	}
}

// meta 本版未实现：必须**安全降级到等价去重**（语义相同、只是省得更少），
// 而不是静默假装做了、也不是直接透传。
func TestResolveToolDrawerMode_MetaDegradesToDedupe(t *testing.T) {
	off := false
	SetToolDrawerEnabled(&off)

	mode := ResolveToolDrawerMode(requestWithToolDrawerHeader("meta"))
	assert.Equal(t, ToolDrawerModeDedupe, mode,
		"meta 未实现时必须降级为等价去重，而不是 off（丢了调用方的意图）")
	assert.NotEqual(t, ToolDrawerModeOff, mode)
}

// 未知取值 → fail-safe 落到全局开关语义，绝不因为一个拼错的头改变行为。
func TestResolveToolDrawerMode_UnknownValueFollowsGlobalSwitch(t *testing.T) {
	setToolDrawerGlobal(t, true)
	assert.Equal(t, ToolDrawerModeDedupe, ResolveToolDrawerMode(requestWithToolDrawerHeader("bogus")),
		"未知取值在有全局开关时应沿用全局")

	off := false
	SetToolDrawerEnabled(&off)
	assert.Equal(t, ToolDrawerModeOff, ResolveToolDrawerMode(requestWithToolDrawerHeader("bogus")),
		"未知取值在全局关时应保持不动")
}

func TestResolveToolDrawerMode_NilContextIsSafe(t *testing.T) {
	setToolDrawerGlobal(t, true)
	assert.Equal(t, ToolDrawerModeDedupe, ResolveToolDrawerMode(nil),
		"nil context 不得 panic，按全局开关处理")
}

// dedupe 的核心契约：**语义等价** —— 保留的工具与输入中「按 name/指纹去重后的首现」
// 完全一致，且顺序不变。这是"零语义风险"这句话的可验证形式。
func TestDedupToolDefsIsSemanticallyEquivalent(t *testing.T) {
	defs := []ToolDef{
		{Name: "alpha", Fingerprint: "fp-alpha", ArgsBytes: 100},
		{Name: "beta", Fingerprint: "fp-beta", ArgsBytes: 200},
		{Name: "alpha", Fingerprint: "fp-alpha", ArgsBytes: 100},    // 同 name 同指纹
		{Name: "alpha", Fingerprint: "fp-alpha-v2", ArgsBytes: 120}, // 同 name，不同指纹 → 仍应被同 name 规则去掉
		{Name: "gamma", Fingerprint: "fp-beta", ArgsBytes: 200},     // 同指纹，不同 name → 按指纹规则去掉
		{Name: "delta", Fingerprint: "fp-delta", ArgsBytes: 300},
	}

	keep := DedupToolDefs(defs)
	require.NotEmpty(t, keep)

	kept := make([]ToolDef, 0, len(keep))
	for _, idx := range keep {
		require.GreaterOrEqual(t, idx, 0)
		require.Less(t, idx, len(defs))
		kept = append(kept, defs[idx])
	}

	// 1) 保留的是首现顺序的子序列
	assert.Equal(t, []string{"alpha", "beta", "delta"}, toolNames(kept))
	// 2) 保留项的指纹与原始定义逐字一致（没有被"改写"）
	assert.Equal(t, []string{"fp-alpha", "fp-beta", "fp-delta"}, toolFingerprints(kept))
	// 3) 每一个被丢掉的定义，都能在保留集合里找到同 name 或同指纹的等价项
	for i, d := range defs {
		if slicesContains(keep, i) {
			continue
		}
		equivalent := false
		for _, k := range kept {
			if k.Name == d.Name || (d.Fingerprint != "" && k.Fingerprint == d.Fingerprint) {
				equivalent = true
				break
			}
		}
		assert.Truef(t, equivalent, "被丢弃的定义 %q(fp=%s) 在保留集合里找不到等价项 —— 去重不是等价变换", d.Name, d.Fingerprint)
	}
}

func TestDedupToolDefsKeepsEverythingWhenNothingIsDuplicate(t *testing.T) {
	defs := []ToolDef{
		{Name: "a", Fingerprint: "fa", ArgsBytes: 1},
		{Name: "b", Fingerprint: "fb", ArgsBytes: 2},
	}
	keep := DedupToolDefs(defs)
	assert.Len(t, keep, 2, "无重复时必须一个都不丢")
}

func toolNames(defs []ToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

func toolFingerprints(defs []ToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Fingerprint)
	}
	return out
}

func slicesContains(haystack []int, needle int) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// 兜底：确认工具抽屉的收益度量在真实去重后确实增长（不是"声明了但没人写"）。
func TestToolDrawerSavingsAreRecorded(t *testing.T) {
	ResetToolDrawerSavingsForTest()
	t.Cleanup(ResetToolDrawerSavingsForTest)

	before := ToolDrawerSavings()
	RecordToolDrawerSavings(0) // 没省到东西 → 不该计入
	assert.Equal(t, before, ToolDrawerSavings())

	RecordToolDrawerSavings(128)
	after := ToolDrawerSavings()
	assert.EqualValues(t, before.DedupedRequests+1, after.DedupedRequests)
	assert.EqualValues(t, before.SavedBytes+128, after.SavedBytes)
}
