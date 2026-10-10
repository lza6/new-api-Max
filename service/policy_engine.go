package service

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
)

// T10 策略引擎（Noveum Nova Guard 迁移）。
//
// 把「护栏 / 预算 / 限流」三类判定收敛到一个**统一策略中心**，支持两种模式：
//   - shadow（影子/观察）：只记录"本应拦截"而不真的拦截——用于上线前评估策略影响。
//   - enforce（执行）：命中即拦截并返回决策原因。
//
// 设计原则（复用优先，零重造）：
//   - 不新建持久化表；策略来源为既有配置（relay_setting 限流档位、用户额度）与
//     环境变量声明的全局规则。
//   - 判定为**纯函数**（Evaluate），返回结构化 PolicyDecision，调用方决定是否执行。
//   - 进程内计数（观测命中分布），无外部依赖。
//
// 开关：POLICY_ENGINE_MODE=off|shadow|enforce（默认 off，零行为变化）。

// PolicyMode 策略引擎模式。
type PolicyMode string

const (
	PolicyModeOff     PolicyMode = "off"     // 关闭：不评估
	PolicyModeShadow  PolicyMode = "shadow"  // 影子：评估并记录，不拦截
	PolicyModeEnforce PolicyMode = "enforce" // 执行：命中即拦截
)

// PolicyKind 策略类别。
type PolicyKind string

const (
	PolicyKindGuardrail PolicyKind = "guardrail" // 内容护栏（如禁词/尺寸上限）
	PolicyKindBudget    PolicyKind = "budget"    // 预算（额度/单请求上限）
	PolicyKindRate      PolicyKind = "rate"      // 限流（RPM/并发）
)

// PolicyAction 决策动作。
type PolicyAction string

const (
	PolicyAllow PolicyAction = "allow"
	PolicyBlock PolicyAction = "block"
	PolicyWarn  PolicyAction = "warn"
)

// PolicyInput 一次策略评估的输入（由调用方从请求上下文抽取，纯数据）。
type PolicyInput struct {
	UserID   int
	Group    string
	Model    string
	IsStream bool
	// 预算类
	UserRemainingQuota int64 // 用户剩余额度（分/额度单位；<0 表示未知）
	RequestEstimate    int64 // 本次预估消耗
	// 护栏类
	PromptChars   int
	MaxPromptChar int // 0=不限制
	// 限流类
	ConcurrencyLimit   int // 0=不限
	CurrentConcurrency int
	RpmLimit           int
	CurrentRpm         int
}

// PolicyDecision 一次策略评估的结果。
type PolicyDecision struct {
	Action   PolicyAction `json:"action"`
	Kind     PolicyKind   `json:"kind,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	Rule     string       `json:"rule,omitempty"`
	Enforced bool         `json:"enforced"` // 是否真的拦截（enforce 模式且 block）
}

// policyModeEnvDefault 策略模式 env 默认值（POLICY_ENGINE_MODE=off|shadow|enforce）。
// 值来源：管理端「实验功能」页持久化配置 > env POLICY_ENGINE_MODE（默认 "off"）。
var policyModeEnvDefault = common.GetEnvOrDefaultString(common.FlagPolicyEngineMode, "off")

func parsePolicyMode(s string) PolicyMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "shadow":
		return PolicyModeShadow
	case "enforce":
		return PolicyModeEnforce
	default:
		return PolicyModeOff
	}
}

// SetPolicyMode 测试用覆盖；nil 恢复 env 默认。
func SetPolicyMode(m *PolicyMode) {
	if m == nil {
		common.SetFeatureFlagOverride(common.FlagPolicyEngineMode, nil)
		return
	}
	raw := string(*m)
	common.SetFeatureFlagOverride(common.FlagPolicyEngineMode, &raw)
}

// PolicyModeValue 返回当前策略模式。
func PolicyModeValue() PolicyMode {
	return parsePolicyMode(common.FeatureFlagString(common.FlagPolicyEngineMode, policyModeEnvDefault))
}

// PolicyEngineEnabled 报告策略引擎是否启用（非 off）。
func PolicyEngineEnabled() bool { return PolicyModeValue() != PolicyModeOff }

// policyCounters 策略命中计数（进程内观测）。
var policyCounters = struct {
	sync.Mutex
	m map[string]int64
}{m: make(map[string]int64)}

func recordPolicyHit(kind PolicyKind, action PolicyAction) {
	policyCounters.Lock()
	policyCounters.m[string(kind)+":"+string(action)]++
	policyCounters.Unlock()
}

// PolicySnapshot 返回策略命中计数快照（观测/指标）。
func PolicySnapshot() map[string]int64 {
	policyCounters.Lock()
	defer policyCounters.Unlock()
	out := make(map[string]int64, len(policyCounters.m))
	for k, v := range policyCounters.m {
		out[k] = v
	}
	return out
}

// ResetPolicyCountersForTest 清空计数（测试用）。
func ResetPolicyCountersForTest() {
	policyCounters.Lock()
	policyCounters.m = make(map[string]int64)
	policyCounters.Unlock()
}

// Evaluate 评估一次请求的完整策略集，返回**最强**决策（block > warn > allow）。
// 纯函数（除进程内计数），可在 shadow 模式下安全调用。
// 调用方据 Enforced 字段与当前模式决定是否真正拦截。
func Evaluate(in PolicyInput) PolicyDecision {
	if !PolicyEngineEnabled() {
		return PolicyDecision{Action: PolicyAllow}
	}
	decision := PolicyDecision{Action: PolicyAllow}

	// 按「护栏 → 预算 → 限流」顺序评估，命中 block 立即返回（短路）。
	if d := evalGuardrail(in); d.Action != PolicyAllow {
		d.Enforced = PolicyModeValue() == PolicyModeEnforce && d.Action == PolicyBlock
		recordPolicyHit(d.Kind, d.Action)
		if d.Action == PolicyBlock {
			return d
		}
		decision = d
	}
	if d := evalBudget(in); d.Action != PolicyAllow {
		d.Enforced = PolicyModeValue() == PolicyModeEnforce && d.Action == PolicyBlock
		recordPolicyHit(d.Kind, d.Action)
		if d.Action == PolicyBlock {
			return d
		}
		if decision.Action == PolicyAllow {
			decision = d
		}
	}
	if d := evalRate(in); d.Action != PolicyAllow {
		d.Enforced = PolicyModeValue() == PolicyModeEnforce && d.Action == PolicyBlock
		recordPolicyHit(d.Kind, d.Action)
		if d.Action == PolicyBlock {
			return d
		}
		if decision.Action == PolicyAllow {
			decision = d
		}
	}
	return decision
}

// evalGuardrail 内容护栏：prompt 超过配置上限 → block。
func evalGuardrail(in PolicyInput) PolicyDecision {
	if in.MaxPromptChar > 0 && in.PromptChars > in.MaxPromptChar {
		return PolicyDecision{
			Action: PolicyBlock,
			Kind:   PolicyKindGuardrail,
			Rule:   "prompt_size",
			Reason: "prompt exceeds configured maximum size",
		}
	}
	return PolicyDecision{Action: PolicyAllow}
}

// evalBudget 预算：单请求预估 > 用户剩余额度 → block。
func evalBudget(in PolicyInput) PolicyDecision {
	if in.UserRemainingQuota >= 0 && in.RequestEstimate > 0 && in.RequestEstimate > in.UserRemainingQuota {
		return PolicyDecision{
			Action: PolicyBlock,
			Kind:   PolicyKindBudget,
			Rule:   "insufficient_quota",
			Reason: "estimated request cost exceeds remaining user quota",
		}
	}
	return PolicyDecision{Action: PolicyAllow}
}

// evalRate 限流：超过并发或 RPM 上限 → block。
func evalRate(in PolicyInput) PolicyDecision {
	if in.ConcurrencyLimit > 0 && in.CurrentConcurrency > in.ConcurrencyLimit {
		return PolicyDecision{
			Action: PolicyBlock,
			Kind:   PolicyKindRate,
			Rule:   "concurrency",
			Reason: "user concurrency limit exceeded",
		}
	}
	if in.RpmLimit > 0 && in.CurrentRpm > in.RpmLimit {
		return PolicyDecision{
			Action: PolicyBlock,
			Kind:   PolicyKindRate,
			Rule:   "rpm",
			Reason: "user RPM limit exceeded",
		}
	}
	return PolicyDecision{Action: PolicyAllow}
}

// policyEvalTotal 评估总次数（观测）。
var policyEvalTotal atomic.Int64

// RecordPolicyEval 记录一次评估调用（供 /metrics 观测）。
func RecordPolicyEval() { policyEvalTotal.Add(1) }

// PolicyEvalTotal 返回评估总次数。
func PolicyEvalTotal() int64 { return policyEvalTotal.Load() }
