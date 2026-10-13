// Package service — G4 智能路由 v2（阶段 A：shadow）。
//
// 本文件实现指南 §6.2.1 的**阶段 A**：把复杂度档位映射到候选模型，并**只记录**
// 「本应路由到哪」，不改实际选渠道。阶段 B（enforce）在开关与配置就绪后才启用。
//
// 为什么必须先 shadow：复杂度打分是**规则**而非模型，必然有误判。直接 enforce 会
// 出现「本该用便宜模型的简单请求被路由到贵模型」这类成本事故，而用户只会看到
// 账单变高。指南 §6.3 的验收门要求「shadow 有数据才 enforce」。
package service

import (
	"slices"

	"github.com/lza6/new-api-Max/setting/routing_setting"
)

// RoutingShadowDecision 一次复杂度路由的**影子决策**（不执行）。
type RoutingShadowDecision struct {
	// Tier 本次请求的复杂度档位。
	Tier string `json:"tier"`
	// Configured 管理员为该档位配置的候选模型（原样，未过滤）。
	Configured []string `json:"configured,omitempty"`
	// Resolved 经**可用性校验**后真正可用于本用户的候选模型。
	Resolved []string `json:"resolved,omitempty"`
	// Rejected 因越权/不可用被剔除的候选，附原因（便于校准配置）。
	Rejected []RoutingRejection `json:"rejected,omitempty"`
	// WouldUse 影子决策「本应选用的模型」（Resolved 的首项；空 = 无决策）。
	WouldUse string `json:"would_use,omitempty"`
	// Reason 决策摘要，直接进日志，供 shadow 期统计误报率。
	Reason string `json:"reason"`
}

// RoutingRejection 记录一个候选被剔除的原因。
type RoutingRejection struct {
	Model  string `json:"model"`
	Reason string `json:"reason"`
}

// 剔除原因（稳定字符串，供日志聚合统计，不要随意改）。
const (
	RoutingRejectNoConfig          = "no_configured_candidates"
	RoutingRejectSameAsRequested   = "same_as_requested_model"
	RoutingRejectNotInUserGroup    = "not_available_in_user_group"
	RoutingRejectAfterEnforceGuard = "rejected_by_enforce_guard"
)

// 模式取值（与 setting/routing_setting 保持一致，避免调用方 import 两个包）。
const (
	RoutingModeOff     = routing_setting.RoutingModeOff
	RoutingModeShadow  = routing_setting.RoutingModeShadow
	RoutingModeEnforce = routing_setting.RoutingModeEnforce
)

// RoutingMode 返回归一化后的路由模式（未知值回退 off）。
func RoutingMode() string {
	return routing_setting.GetRoutingMode()
}

// RoutingEnabled 报告是否处于任何非 off 模式。
func RoutingEnabled() bool {
	return RoutingMode() != RoutingModeOff
}

// ResolveRoutingShadow 计算本次请求的影子决策。
//
// 硬约束（指南 §6.2.1，缺一不可）：
//  1. **不得越过用户可用分组/模型白名单** —— `isModelAvailable` 由调用方注入，
//     复用既有分组校验，不在本函数里重写一套判断。
//  2. **不得改变计费口径** —— 本函数只返回决策，不触碰 RelayInfo 的模型字段；
//     阶段 B 也必须按**最终实际模型**计费。
//  3. **无可用候选时回退原模型** —— 返回空的 WouldUse 而不是让调用方变成
//     「无可用渠道」；调用方据此保持原样。
//
// 返回的决策只用于日志与统计；调用方**不得**据此改变请求。
func ResolveRoutingShadow(
	tier ComplexityTier,
	requestedModel string,
	isModelAvailable func(model string) bool,
) RoutingShadowDecision {
	decision := RoutingShadowDecision{Tier: string(tier)}

	configured := routing_setting.GetTierModels(string(tier))
	if len(configured) == 0 {
		decision.Reason = RoutingRejectNoConfig
		return decision
	}
	decision.Configured = configured

	for _, candidate := range configured {
		// 候选就是本次请求的模型：不算「改路由」，跳过（否则日志会被噪声淹没）。
		if candidate == requestedModel {
			decision.Rejected = append(decision.Rejected, RoutingRejection{
				Model: candidate, Reason: RoutingRejectSameAsRequested,
			})
			continue
		}
		// 白名单/分组校验：不可用即剔除，绝不因为「路由想要」而放行。
		if isModelAvailable != nil && !isModelAvailable(candidate) {
			decision.Rejected = append(decision.Rejected, RoutingRejection{
				Model: candidate, Reason: RoutingRejectNotInUserGroup,
			})
			continue
		}
		decision.Resolved = append(decision.Resolved, candidate)
	}

	if len(decision.Resolved) == 0 {
		// 指南要求：无可用候选 → 回退原模型并记录，不得变成「无可用渠道」。
		decision.Reason = RoutingRejectNotInUserGroup
		return decision
	}

	decision.WouldUse = decision.Resolved[0]
	decision.Reason = "shadow_only"
	return decision
}

// ApplyEnforceGuard 是阶段 B 的**唯一**注入点：给定影子决策，返回真正要使用的模型。
//
// 当前实现是恒等函数（始终返回 requestedModel）——阶段 B 启用后才会在
// 模式为 enforce 且已有 shadow 数据时返回 decision.WouldUse。
//
// 之所以现在就把注入点留出来而不是等到启用时再加：调用点在 `controller/relay.go`
// 的请求处理主链路上，改动它需要回归整个 relay 路径；把「决策」与「执行」在代码
// 结构上分开，阶段 B 只需替换本函数的实现，无需再动主链路。
func ApplyEnforceGuard(decision RoutingShadowDecision, requestedModel string) string {
	if RoutingMode() != RoutingModeEnforce {
		return requestedModel
	}
	// enforce 模式下仍要求候选经过白名单校验（ResolveRoutingShadow 已保证），
	// 且必须是本用户可用分组内的模型。
	if decision.WouldUse == "" {
		return requestedModel
	}
	return decision.WouldUse
}

// RoutingShadowRejectionReasons 返回决策中所有剔除原因（去重、稳定顺序），
// 供日志字段与统计聚合使用。
func RoutingShadowRejectionReasons(decision RoutingShadowDecision) []string {
	if len(decision.Rejected) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(decision.Rejected))
	out := make([]string, 0, len(decision.Rejected))
	for _, r := range decision.Rejected {
		if _, ok := seen[r.Reason]; ok {
			continue
		}
		seen[r.Reason] = struct{}{}
		out = append(out, r.Reason)
	}
	slices.Sort(out)
	return out
}
