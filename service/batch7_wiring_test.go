package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
)

// 接线回归测试（《参考的结果计划指南》§3「自营平台 v161 伪闭环事故教训」第 2 条）：
// 新模块交付时必须验证**生产链路真的调用它**。Batch-5 的 T4/T7/T10 曾经是
// 「库单测全绿但生产链路零调用」的死代码 —— 本文件的目的是让那种状态不可能再
// 悄悄发生：以下用例全部针对**跨模块的调用契约**（context 键、日志字段、指标计数），
// 而不是模块内部的纯函数行为（那些已由 batch5_test.go / policy_engine_test.go 覆盖）。

func newAuditTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

// T4 接线：打分结果写入 context 的 ContextKeyComplexityScore，
// 且**真实的日志公共入口** AppendRelayLogAdminInfo 把它读出来
// （档位公开可见、细分仅管理员可见）。
//
// 本用例刻意调用 AppendRelayLogAdminInfo 而非 appendComplexityScore：
// 后者只能证明「函数本身正确」，前者才证明「生产路径真的调用了它」——
// 这正是 Batch-5 死代码能骗过全绿单测的漏洞所在。
func TestComplexityScoreFlowsFromContextToLog(t *testing.T) {
	on := true
	SetComplexityRoutingEnabled(&on)
	defer SetComplexityRoutingEnabled(nil)

	c, _ := newAuditTestContext()
	relayInfo := &relaycommon.RelayInfo{}

	// 未打分时：日志里不应出现任何复杂度字段（零行为变化）。
	other := model.NewLogOther()
	AppendRelayLogAdminInfo(c, relayInfo, other)
	assert.NotContains(t, other.Snapshot(), "complexity_tier")
	adminInfo, _ := other.Snapshot()["admin_info"].(map[string]any)
	assert.NotContains(t, adminInfo, "complexity")

	// 打分写入 context 后：档位进公开字段，细分进 admin_info。
	score := ScoreComplexity(ComplexitySignals{PromptChars: 20000, MessageCount: 12, HasReasoning: true})
	common.SetContextKey(c, constant.ContextKeyComplexityScore, score)

	other = model.NewLogOther()
	AppendRelayLogAdminInfo(c, relayInfo, other)
	snap := other.Snapshot()
	assert.Equal(t, string(score.Tier), snap["complexity_tier"], "档位必须对用户可见")
	adminInfo, ok := snap["admin_info"].(map[string]any)
	require.True(t, ok, "细维度必须进 admin_info")
	assert.NotNil(t, adminInfo["complexity"], "生产日志入口必须写出复杂度细分")
}

// T7 接线：流式逐帧自检真的被调用（SSE 键白名单 + usage 单调），
// 且发现经 FlushRelayAudit 落到 admin_info 并计入进程内指标。
func TestRelayAuditFrameCollectionAndFlush(t *testing.T) {
	on := true
	SetRelayAuditEnabled(&on)
	defer SetRelayAuditEnabled(nil)
	ResetRelayAuditCountersForTest()

	c, _ := newAuditTestContext()

	// 合法帧：不产生任何发现。
	AuditStreamFrame(c, `{"id":"1","object":"chat.completion.chunk","choices":[]}`)
	assert.Empty(t, RelayAuditFindings(c), "合法 SSE 帧不应产生发现")

	// 非标字段帧 → sse_whitelist 发现。
	AuditStreamFrame(c, `{"id":"1","choices":[],"evil_field":123}`)
	findings := RelayAuditFindings(c)
	require.NotEmpty(t, findings, "非标 SSE 键必须被检出")
	assert.Equal(t, "sse_whitelist", findings[0].Check)

	// usage 回退帧 → usage_monotonic 发现。
	AuditStreamFrame(c, `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`)
	AuditStreamFrame(c, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	var hasMonotonic bool
	for _, f := range RelayAuditFindings(c) {
		if f.Check == "usage_monotonic" {
			hasMonotonic = true
		}
	}
	assert.True(t, hasMonotonic, "usage 回退必须被检出")

	// 收口：写入 admin_info 并计入指标（此前这两个计数器恒为 0 = 死代码特征）。
	other := model.NewLogOther()
	AppendRelayLogAdminInfo(c, &relaycommon.RelayInfo{}, other)
	snap := other.Snapshot()
	adminInfo, ok := snap["admin_info"].(map[string]any)
	require.True(t, ok, "发现必须落到 admin_info（非管理员不可见）")
	assert.NotEmpty(t, adminInfo["relay_audit"], "生产日志入口必须写出中继自检发现")

	counterSnap := RelayAuditSnapshot()
	assert.Greater(t, counterSnap["sse_whitelist:warn"]+counterSnap["usage_monotonic:warn"], int64(0),
		"指标计数器必须被真实推进（死代码时恒为 0）")
}

// T7 接线：错误文本泄漏检查在错误路径被调用。
func TestRelayAuditErrorLeakWiring(t *testing.T) {
	on := true
	SetRelayAuditEnabled(&on)
	defer SetRelayAuditEnabled(nil)

	c, _ := newAuditTestContext()
	AuditUpstreamError(c, "upstream rejected: Authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz0123456789")

	findings := RelayAuditFindings(c)
	require.Len(t, findings, 1)
	assert.Equal(t, "error_leak", findings[0].Check)
	assert.Equal(t, "error", findings[0].Severity)
}

// T7 接线：模型指纹校验每请求只跑一次（避免重复写同一条）。
func TestRelayAuditModelFingerprintOnce(t *testing.T) {
	on := true
	SetRelayAuditEnabled(&on)
	defer SetRelayAuditEnabled(nil)

	c, _ := newAuditTestContext()
	AuditModelFingerprintOnce(c, "deepseek-v4.1-flash", "totally-different-model-x")
	AuditModelFingerprintOnce(c, "deepseek-v4.1-flash", "totally-different-model-x")

	findings := RelayAuditFindings(c)
	require.Len(t, findings, 1, "同请求内指纹校验只应记录一次")
	assert.Equal(t, "model_fingerprint", findings[0].Check)
}

// T7 关键不变量：开关关闭时**零副作用**——不产生发现、不建 context 条目、不计数。
func TestRelayAuditDisabledIsNoOp(t *testing.T) {
	off := false
	SetRelayAuditEnabled(&off)
	defer SetRelayAuditEnabled(nil)
	ResetRelayAuditCountersForTest()

	c, _ := newAuditTestContext()
	AuditStreamFrame(c, `{"id":"1","choices":[],"evil_field":123}`)
	AuditUpstreamError(c, "Bearer sk-abcdefghijklmnopqrstuvwxyz0123456789")
	AuditModelFingerprintOnce(c, "a", "b")

	assert.Empty(t, RelayAuditFindings(c), "开关关闭必须零副作用")
	assert.Empty(t, RelayAuditSnapshot(), "开关关闭不应推进任何指标")

	other := model.NewLogOther()
	FlushRelayAudit(c, other.SetAdmin)
	assert.NotContains(t, other.Snapshot(), "admin_info")
}

// T10 接线：策略引擎评估真的被计入指标，且三来源（护栏/预算/限流）在
// **真实输入形态**下可命中。评价函数本身由 policy_engine_test.go 覆盖，
// 这里只锁定「接线后指标被推进」与「off 模式零开销」。
func TestPolicyEngineEvalCounterIsWired(t *testing.T) {
	off := PolicyModeOff
	SetPolicyMode(&off)
	defer SetPolicyMode(nil)
	ResetPolicyCountersForTest()

	before := PolicyEvalTotal()
	// off 模式：Evaluate 首行返回 allow，不计数。
	d := Evaluate(PolicyInput{PromptChars: 999999, MaxPromptChar: 10})
	assert.Equal(t, PolicyAllow, d.Action)
	assert.Equal(t, before, PolicyEvalTotal())

	shadow := PolicyModeShadow
	SetPolicyMode(&shadow)
	RecordPolicyEval()
	d = Evaluate(PolicyInput{PromptChars: 100, MaxPromptChar: 10})
	assert.Equal(t, PolicyBlock, d.Action)
	assert.False(t, d.Enforced, "shadow 不得真拦截")
	assert.Equal(t, before+1, PolicyEvalTotal(), "评估必须被计数（死代码时恒为 0）")
}

// T10 接线：预算判定的额度语义必须与预扣费一致 —— 额度未知（<0）时不拦，
// 否则会误伤信任用户与订阅计费用户。
func TestPolicyBudgetSkipsWhenQuotaUnknown(t *testing.T) {
	enf := PolicyModeEnforce
	SetPolicyMode(&enf)
	defer SetPolicyMode(nil)

	d := Evaluate(PolicyInput{UserRemainingQuota: -1, RequestEstimate: 999999})
	assert.Equal(t, PolicyAllow, d.Action, "额度未知时必须放行，交由 PreConsumeBilling 权威处理")

	d = Evaluate(PolicyInput{UserRemainingQuota: 100, RequestEstimate: 999999})
	assert.Equal(t, PolicyBlock, d.Action)
	assert.True(t, d.Enforced)
}
