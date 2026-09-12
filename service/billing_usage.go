package service

import (
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relaykit/dto"
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
// other.explain = {facts: [{label, value}]}，前端"费用解释"卡片据此渲染。
// facts 全部来自本次请求已有的计费数据（零额外计算）；不含 key/上游 URL。
func appendBillingExplain(other *model.LogOther, summary textQuotaSummary) {
	if other == nil {
		return
	}
	facts := make([]map[string]any, 0, 8)
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
	other.SetPublic("explain", map[string]any{"facts": facts})
}

func usageFromBillingUsage(usage *dto.Usage) (*dto.Usage, bool) {
	if usage == nil || usage.BillingUsage == nil {
		return nil, false
	}
	return usage.BillingUsage.CanonicalUsage()
}
