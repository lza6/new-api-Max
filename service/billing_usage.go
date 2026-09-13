package service

import (
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relaykit/dto"

	"github.com/gin-gonic/gin"
)

const (
	usageBillingPathLocal              = "local"
	usageBillingPathUpstream           = "upstream"
	usageBillingPathOpenAI             = "billing-usage-openai"
	usageBillingPathOpenAIEstimated    = "billing-usage-openai-estimated"
	usageBillingPathAnthropic          = "billing-usage-anthropic"
	usageBillingPathAnthropicEstimated = "billing-usage-anthropic-estimated"
	usageBillingPathGemini             = "billing-usage-gemini"
	usageBillingPathGeminiEstimated    = "billing-usage-gemini-estimated"
)

func effectiveBillingUsage(usage *dto.Usage) *dto.Usage {
	if billingUsage, ok := usageFromBillingUsage(usage); ok {
		return billingUsage
	}
	return usage
}

func usageBillingPathForLog(isLocalCountTokens bool, usage *dto.Usage) string {
	effectiveUsage, ok := usageFromBillingUsage(usage)
	if !ok {
		if isLocalCountTokens {
			return usageBillingPathLocal
		}
		return usageBillingPathUpstream
	}

	switch effectiveUsage.UsageSemantic {
	case dto.BillingUsageSemanticOpenAI:
		if usage.BillingUsage.Estimated {
			return usageBillingPathOpenAIEstimated
		}
		return usageBillingPathOpenAI
	case dto.BillingUsageSemanticAnthropic:
		if usage.BillingUsage.Estimated {
			return usageBillingPathAnthropicEstimated
		}
		return usageBillingPathAnthropic
	case dto.BillingUsageSemanticGemini:
		if usage.BillingUsage.Estimated {
			return usageBillingPathGeminiEstimated
		}
		return usageBillingPathGemini
	}

	return usageBillingPathUpstream
}

func appendUsageBillingPathForLog(other *model.LogOther, isLocalCountTokens bool, usage *dto.Usage) {
	if other == nil {
		return
	}
	other.SetAdmin("usage_billing_path", usageBillingPathForLog(isLocalCountTokens, usage))
}

// appendBillingExplain B5-1 解释性日志：把计费要素结构化为
// other.explain = {facts: [{label, value}], inferences: [{text, kind}]}，
// 前端"费用解释"卡片据此渲染。facts 全部来自本次请求已有的计费数据
// （零额外计算）；不含 key/上游 URL。
func appendBillingExplain(ctx *gin.Context, other *model.LogOther, summary textQuotaSummary) {
	if other == nil {
		return
	}
	facts := make([]map[string]any, 0, 10)
	fact := func(label string, value any) {
		facts = append(facts, map[string]any{"label": label, "value": value})
	}
	fact("prompt_tokens", summary.PromptTokens)
	if summary.CacheTokens > 0 {
		fact("cached_tokens", summary.CacheTokens)
	}
	fact("completion_tokens", summary.CompletionTokens)
	if summary.ModelPrice > 0 {
		fact("model_price", summary.ModelPrice)
	} else {
		fact("model_ratio", summary.ModelRatio)
		if summary.CompletionRatio > 0 && summary.CompletionRatio != 1 {
			fact("completion_ratio", summary.CompletionRatio)
		}
		if summary.CacheTokens > 0 {
			fact("cache_ratio", summary.CacheRatio)
		}
	}
	fact("group_ratio", summary.GroupRatio)

	inferences := make([]map[string]any, 0, 4)
	infer := func(text, kind string) {
		inferences = append(inferences, map[string]any{"text": text, "kind": kind})
	}

	// Tiered（表达式计费）路径：matched_tier 来自结算结果，命中说明逐级直接
	// 解释定价，零额外计算；固定单价场景再追加一条按请求计费的推断。
	switch {
	case summary.FixedPriceBilling && summary.FixedPriceValue > 0:
		fact("fixed_price", summary.FixedPriceValue)
		if summary.FixedPriceTier != "" {
			fact("tier_matched", summary.FixedPriceTier)
		}
		infer("该请求按固定单价计费（按请求计价，与 Token 用量无关）。", "pricing")
	case summary.MatchedTier != "" && summary.BillingUnit != "":
		fact("tier_matched", summary.MatchedTier)
		fact("billing_unit", summary.BillingUnit)
		infer("命中阶梯价 "+summary.MatchedTier+" 档，费用按该档单价计算。", "pricing")
	case summary.MatchedTier != "":
		fact("tier_matched", summary.MatchedTier)
	}

	// 渠道选择事实：本次请求经过（候选过/尝试过）的渠道数量。来源是
	// CacheGetRandomSatisfiedChannel 在 context 中记录的 id 列表，零额外计算。
	if ctx != nil {
		raw, _ := common.GetContextKey(ctx, constant.ContextKeyRoutingExplainedChannels)
		explainedIDs, _ := raw.([]int)
		if len(explainedIDs) > 0 {
			fact("channels_considered", len(explainedIDs))
		}
	}

	other.SetPublic("explain", map[string]any{"facts": facts, "inferences": inferences})
}

func usageFromBillingUsage(usage *dto.Usage) (*dto.Usage, bool) {
	if usage == nil || usage.BillingUsage == nil {
		return nil, false
	}
	return usage.BillingUsage.CanonicalUsage()
}
