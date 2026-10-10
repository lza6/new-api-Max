package service

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
)

// T7 中继一致性自检（api-relay-audit 迁移）。
//
// 在请求完成后对「本次中继」做一组**只读一致性检查**，把可疑结果（协议不合法、
// 用量回退、错误信息泄漏上游密钥、响应模型与请求不符）标记出来，供排障与质量
// 观测。纯函数 + 进程内计数，不阻塞请求路径；开关默认关（RELAY_AUDIT_ENABLED）。
//
// 四类检查：
//  1. SSE 白名单：流式事件名/字段是否在允许集合内（防上游返回非标事件污染下游）。
//  2. usage 单调：completion/prompt token 在流式累计中不得减少（回退=上游异常）。
//  3. 错误不泄漏：错误文本不得包含上游 key、Bearer token、内部堆栈特征。
//  4. 渠道指纹：上游返回的 model 名应与请求模型同族（防串号/串渠道）。

// RelayAuditFinding 一条自检发现。
type RelayAuditFinding struct {
	Check    string `json:"check"`    // sse_whitelist | usage_monotonic | error_leak | model_fingerprint
	Severity string `json:"severity"` // warn | error
	Detail   string `json:"detail"`
}

// relayAuditCounters 各检查的命中计数（进程内，供 /metrics 与排障）。
var relayAuditCounters = struct {
	sync.Mutex
	m map[string]int64
}{m: make(map[string]int64)}

// RelayAuditEnabled 开关：RELAY_AUDIT_ENABLED=true 开启。默认关（零行为变化）。
// 值来源：管理端「实验功能」页持久化配置 > env RELAY_AUDIT_ENABLED（默认 false）。
var relayAuditEnvDefault = common.GetEnvOrDefaultBool(common.FlagRelayAuditEnabled, false)

// SetRelayAuditEnabled 测试用覆盖；nil 恢复 env 默认。
func SetRelayAuditEnabled(v *bool) {
	common.SetFeatureFlagOverride(common.FlagRelayAuditEnabled, common.BoolFeatureFlagOverride(v))
}

// RelayAuditEnabled 报告中继自检开关状态。
func RelayAuditEnabled() bool {
	return common.FeatureFlagValue(common.FlagRelayAuditEnabled, relayAuditEnvDefault)
}

var relayAuditTotal atomic.Int64

// RecordRelayAudit 记录一次自检命中（进程内计数）。
func RecordRelayAudit(f RelayAuditFinding) {
	relayAuditTotal.Add(1)
	relayAuditCounters.Lock()
	relayAuditCounters.m[f.Check+":"+f.Severity]++
	relayAuditCounters.Unlock()
}

// RelayAuditSnapshot 返回自检计数快照（排障/指标）。
func RelayAuditSnapshot() map[string]int64 {
	relayAuditCounters.Lock()
	defer relayAuditCounters.Unlock()
	out := make(map[string]int64, len(relayAuditCounters.m))
	for k, v := range relayAuditCounters.m {
		out[k] = v
	}
	return out
}

// ResetRelayAuditCountersForTest 清空计数（测试用）。
func ResetRelayAuditCountersForTest() {
	relayAuditCounters.Lock()
	relayAuditCounters.m = make(map[string]int64)
	relayAuditCounters.Unlock()
	relayAuditTotal.Store(0)
}

// AuditErrorLeak 检查错误文本是否泄漏敏感信息（上游 key / Bearer / 堆栈特征）。
// 返回发现列表（空=干净）。
func AuditErrorLeak(errText string) []RelayAuditFinding {
	if strings.TrimSpace(errText) == "" {
		return nil
	}
	var out []RelayAuditFinding
	if relayAuditErrorLeakRe.MatchString(errText) {
		out = append(out, RelayAuditFinding{
			Check:    "error_leak",
			Severity: "error",
			Detail:   "error message appears to leak an upstream credential or bearer token",
		})
	}
	if strings.Contains(errText, "goroutine ") && strings.Contains(errText, ".go:") {
		out = append(out, RelayAuditFinding{
			Check:    "error_leak",
			Severity: "warn",
			Detail:   "error message appears to include a stack trace",
		})
	}
	return out
}

// relayAuditErrorLeakRe 匹配密钥/令牌特征（sk-... / Bearer ... / ak-... / 明显长 token）。
var relayAuditErrorLeakRe = regexp.MustCompile(`(?i)(sk-[a-z0-9]{16,}|bearer\s+[a-z0-9._-]{20,}|ak-[a-z0-9]{16,}|x-api-key['"\s:=]+[a-z0-9]{16,})`)

// AuditModelFingerprint 检查上游返回模型名是否与请求模型同族。
// 宽松判定：忽略大小写与 "-"/"_"/"." 分隔后，两者共享首个字母数字段即视为同族，
// 或上游名包含请求名的核心 token（如 deepseek）。空 upstreamModel 跳过（未回显）。
func AuditModelFingerprint(requestedModel, upstreamModel string) []RelayAuditFinding {
	if strings.TrimSpace(upstreamModel) == "" || strings.TrimSpace(requestedModel) == "" {
		return nil
	}
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.NewReplacer("-", "", "_", "", ".", "", "/", "", ":", "").Replace(s)
		return s
	}
	reqN, upN := norm(requestedModel), norm(upstreamModel)
	if reqN == "" || upN == "" {
		return nil
	}
	// 同族：任一包含另一的前 4 个字符，或互相包含。
	prefix := reqN
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	if strings.Contains(upN, prefix) || strings.Contains(reqN, firstSegment(upN)) {
		return nil
	}
	return []RelayAuditFinding{{
		Check:    "model_fingerprint",
		Severity: "warn",
		Detail:   "upstream model " + upstreamModel + " does not match requested model family " + requestedModel,
	}}
}

func firstSegment(s string) string {
	if len(s) > 4 {
		return s[:4]
	}
	return s
}

// usageMonotonicState 跟踪单次流式的 usage 单调性。
type usageMonotonicState struct {
	lastPrompt     int
	lastCompletion int
	lastTotal      int
}

// AuditUsageMonotonic 检查 usage 是否单调不减（prompt/completion/total）。
// 传入本次流式已累计的 usage；返回发现（回退=warn）。state 由调用方在单次请求内持有。
func AuditUsageMonotonic(state *usageMonotonicState, prompt, completion, total int) []RelayAuditFinding {
	if state == nil {
		return nil
	}
	var out []RelayAuditFinding
	if prompt < state.lastPrompt || completion < state.lastCompletion || total < state.lastTotal {
		out = append(out, RelayAuditFinding{
			Check:    "usage_monotonic",
			Severity: "warn",
			Detail:   "usage decreased mid-stream (upstream sent inconsistent token counts)",
		})
	}
	if prompt > state.lastPrompt {
		state.lastPrompt = prompt
	}
	if completion > state.lastCompletion {
		state.lastCompletion = completion
	}
	if total > state.lastTotal {
		state.lastTotal = total
	}
	return out
}

// NewUsageMonotonicState 为一次流式请求创建 usage 单调跟踪状态。
func NewUsageMonotonicState() *usageMonotonicState { return &usageMonotonicState{} }

// SSE 白名单：允许的 OpenAI 兼容流式 delta 顶层键（宽松，防明显非标字段）。
var relayAuditAllowedSSEKeys = map[string]struct{}{
	"id": {}, "object": {}, "created": {}, "model": {}, "choices": {},
	"usage": {}, "system_fingerprint": {}, "service_tier": {},
	"prompt_filter_results": {}, "obfuscation": {},
}

// AuditSSEWhitelist 检查一帧 SSE delta 的顶层键是否在白名单内（未知键=warn）。
// keys 由调用方从解码后的 map 提取。空 keys 跳过。
func AuditSSEWhitelist(keys []string) []RelayAuditFinding {
	if len(keys) == 0 {
		return nil
	}
	var unknown []string
	for _, k := range keys {
		if _, ok := relayAuditAllowedSSEKeys[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return []RelayAuditFinding{{
		Check:    "sse_whitelist",
		Severity: "warn",
		Detail:   "unexpected SSE keys: " + strings.Join(unknown, ","),
	}}
}
