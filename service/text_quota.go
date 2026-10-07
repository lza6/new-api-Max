package service

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/pkg/billingexpr"
	perfmetrics "github.com/lza6/new-api-Max/pkg/perf_metrics"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ToolSurchargeItem is one billable tool-call line for consume logs.
type ToolSurchargeItem struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Price float64 `json:"price"`
}

func appendToolSurchargeLogInfo(other *model.LogOther, items []ToolSurchargeItem) {
	if len(items) == 0 {
		return
	}
	other.SetPublic("tool_surcharges", items)
}

type textQuotaSummary struct {
	PromptTokens           int
	CompletionTokens       int
	TotalTokens            int
	CacheTokens            int
	CacheCreationTokens    int
	CacheCreationTokens5m  int
	CacheCreationTokens1h  int
	ImageTokens            int
	AudioTokens            int
	ModelName              string
	TokenName              string
	UseTimeSeconds         int64
	CompletionRatio        float64
	CacheRatio             float64
	ImageRatio             float64
	ModelRatio             float64
	GroupRatio             float64
	ModelPrice             float64
	CacheCreationRatio     float64
	CacheCreationRatio5m   float64
	CacheCreationRatio1h   float64
	Quota                  int
	IsClaudeUsageSemantic  bool
	UsageSemantic          string
	AudioInputPrice        float64
	ToolSurchargeItems     []ToolSurchargeItem
	ToolCallSurchargeQuota decimal.Decimal
	FixedPriceBilling      bool
	// FixedPriceTier 记录按请求计费（fixed-price）落到的 tier 名，来自
	// tiered 结算结果；FixedPriceValue 为该档固定单价。两者仅 FixedPriceBilling
	// 时有效，用于费用解释卡（B5-1）的事实与推断。
	FixedPriceTier  string
	FixedPriceValue float64
	BillingUnit     string
	MatchedTier     string
}

// hasBillableUsage reports whether this request should incur any charge.
// A request can carry zero tokens yet still be billable via a tool-call
// surcharge (e.g. /v1/alpha/search returns no usage but bills one web_search
// call), so token count alone is not sufficient to decide.
func (s *textQuotaSummary) hasBillableUsage() bool {
	return s.FixedPriceBilling || s.TotalTokens > 0 || !s.ToolCallSurchargeQuota.IsZero()
}

func cacheWriteTokensTotal(summary textQuotaSummary) int {
	if summary.CacheCreationTokens5m > 0 || summary.CacheCreationTokens1h > 0 {
		splitCacheWriteTokens := summary.CacheCreationTokens5m + summary.CacheCreationTokens1h
		if summary.CacheCreationTokens > splitCacheWriteTokens {
			return summary.CacheCreationTokens
		}
		return splitCacheWriteTokens
	}
	return summary.CacheCreationTokens
}

func isLegacyClaudeDerivedOpenAIUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) bool {
	if relayInfo == nil || usage == nil {
		return false
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return false
	}
	if usage.UsageSource != "" || usage.UsageSemantic != "" {
		return false
	}
	return usage.ClaudeCacheCreation5mTokens > 0 || usage.ClaudeCacheCreation1hTokens > 0
}

func collectToolSurchargeItem(items []ToolSurchargeItem, name string, count int, modelName string) []ToolSurchargeItem {
	if count <= 0 {
		return items
	}
	price := operation_setting.GetToolPriceForModel(name, modelName)
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return items
	}
	return append(items, ToolSurchargeItem{
		Name:  name,
		Count: count,
		Price: price,
	})
}

func mergeToolSurchargeItems(items []ToolSurchargeItem) []ToolSurchargeItem {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Price < items[j].Price
		}
		return items[i].Name < items[j].Name
	})

	merged := items[:0]
	for _, item := range items {
		lastIndex := len(merged) - 1
		if lastIndex >= 0 &&
			merged[lastIndex].Name == item.Name &&
			merged[lastIndex].Price == item.Price {
			if item.Count > math.MaxInt-merged[lastIndex].Count {
				common.SysError("tool surcharge call count overflow for " + item.Name)
				merged[lastIndex].Count = math.MaxInt
			} else {
				merged[lastIndex].Count += item.Count
			}
			continue
		}
		merged = append(merged, item)
	}
	return merged
}

func calculateTextToolCallSurcharge(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, summary *textQuotaSummary) decimal.Decimal {
	dGroupRatio := decimal.NewFromFloat(summary.GroupRatio)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	var items []ToolSurchargeItem

	if relayInfo.ResponsesUsageInfo != nil {
		for name, tool := range relayInfo.ResponsesUsageInfo.BuiltInTools {
			if tool == nil {
				continue
			}
			items = collectToolSurchargeItem(items, name, tool.CallCount, summary.ModelName)
		}
	}
	if relayInfo.RelayMode != relayconstant.RelayModeResponses &&
		strings.HasSuffix(summary.ModelName, "search-preview") {
		items = collectToolSurchargeItem(items, dto.BuildInToolWebSearchPreview, 1, summary.ModelName)
	}

	items = collectToolSurchargeItem(
		items,
		dto.BuildInToolWebSearch,
		ctx.GetInt("claude_web_search_requests"),
		summary.ModelName,
	)

	if ctx.GetBool("gemini_google_search_call") {
		items = collectToolSurchargeItem(items, dto.BuildInToolGoogleSearch, 1, summary.ModelName)
	}

	summary.ToolSurchargeItems = mergeToolSurchargeItems(items)
	var surcharge decimal.Decimal
	for _, item := range summary.ToolSurchargeItems {
		surcharge = surcharge.Add(decimal.NewFromFloat(item.Price).
			Mul(decimal.NewFromInt(int64(item.Count))).
			Div(decimal.NewFromInt(1000)).
			Mul(dGroupRatio).
			Mul(dQuotaPerUnit))
	}

	return surcharge
}

// noteQuotaClamp records the first quota saturation event onto relayInfo so it
// can later be attached to the consume/task log for admin auditing. First
// non-nil clamp wins (a single request may hit multiple conversions).
func noteQuotaClamp(relayInfo *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || relayInfo == nil {
		return
	}
	if relayInfo.QuotaClamp == nil {
		relayInfo.QuotaClamp = clamp
	}
}

func composeTieredTextQuota(relayInfo *relaycommon.RelayInfo, summary textQuotaSummary, tieredQuota int, tieredResult *billingexpr.TieredResult) int {
	if summary.ToolCallSurchargeQuota.IsZero() {
		return tieredQuota
	}

	if tieredResult != nil {
		if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromFloat(tieredResult.ActualQuotaBeforeGroup).
				Mul(decimal.NewFromFloat(snap.GroupRatio)).
				Add(summary.ToolCallSurchargeQuota))
			noteQuotaClamp(relayInfo, clamp)
			return quota
		}
	}

	// Saturate the final sum, not just the surcharge: tieredQuota can be near
	// MaxQuota and adding the surcharge could push the total past the
	// single-request quota policy bound.
	total, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(int64(tieredQuota)).Add(summary.ToolCallSurchargeQuota),
	)
	noteQuotaClamp(relayInfo, clamp)
	return total
}

// calculateTextQuotaSummary expects a usage already remapped by
// effectiveBillingUsage; PostTextConsumeQuota performs that remap once and shares
// the result with tiered billing, affinity observation and logging.
func calculateTextQuotaSummary(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) textQuotaSummary {
	summary := textQuotaSummary{
		ModelName:            relayInfo.GetBillingModelName(),
		TokenName:            ctx.GetString("token_name"),
		UseTimeSeconds:       time.Now().Unix() - relayInfo.StartTime.Unix(),
		CompletionRatio:      relayInfo.PriceData.CompletionRatio,
		CacheRatio:           relayInfo.PriceData.CacheRatio,
		ImageRatio:           relayInfo.PriceData.ImageRatio,
		ModelRatio:           relayInfo.PriceData.ModelRatio,
		GroupRatio:           relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		ModelPrice:           relayInfo.PriceData.ModelPrice,
		CacheCreationRatio:   relayInfo.PriceData.CacheCreationRatio,
		CacheCreationRatio5m: relayInfo.PriceData.CacheCreation5mRatio,
		CacheCreationRatio1h: relayInfo.PriceData.CacheCreation1hRatio,
		UsageSemantic:        usageSemanticFromUsage(relayInfo, usage),
	}
	summary.IsClaudeUsageSemantic = summary.UsageSemantic == "anthropic"

	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.GetEstimatePromptTokens(),
			CompletionTokens: 0,
			TotalTokens:      relayInfo.GetEstimatePromptTokens(),
		}
	}

	summary.PromptTokens = usage.PromptTokens
	summary.CompletionTokens = usage.CompletionTokens
	summary.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	summary.CacheTokens = usage.PromptTokensDetails.CachedTokens
	summary.CacheCreationTokens = usage.PromptTokensDetails.CacheCreationTokensTotal()
	summary.CacheCreationTokens5m = usage.ClaudeCacheCreation5mTokens
	summary.CacheCreationTokens1h = usage.ClaudeCacheCreation1hTokens
	summary.ImageTokens = usage.PromptTokensDetails.ImageTokens
	summary.AudioTokens = usage.PromptTokensDetails.AudioTokens
	legacyClaudeDerived := isLegacyClaudeDerivedOpenAIUsage(relayInfo, usage)
	isOpenRouterClaudeBilling := relayInfo.ChannelMeta != nil &&
		relayInfo.ChannelType == constant.ChannelTypeOpenRouter &&
		summary.IsClaudeUsageSemantic

	if isOpenRouterClaudeBilling {
		summary.PromptTokens -= summary.CacheTokens
		isUsingCustomSettings := relayInfo.PriceData.UsePrice || hasCustomModelRatio(summary.ModelName, relayInfo.PriceData.ModelRatio)
		if summary.CacheCreationTokens == 0 && relayInfo.PriceData.CacheCreationRatio != 1 && usage.Cost != 0 && !isUsingCustomSettings {
			maybeCacheCreationTokens := CalcOpenRouterCacheCreateTokens(*usage, relayInfo.PriceData)
			if maybeCacheCreationTokens >= 0 && summary.PromptTokens >= maybeCacheCreationTokens {
				summary.CacheCreationTokens = maybeCacheCreationTokens
			}
		}
		summary.PromptTokens -= summary.CacheCreationTokens
	}

	dPromptTokens := decimal.NewFromInt(int64(summary.PromptTokens))
	dCacheTokens := decimal.NewFromInt(int64(summary.CacheTokens))
	dImageTokens := decimal.NewFromInt(int64(summary.ImageTokens))
	dAudioTokens := decimal.NewFromInt(int64(summary.AudioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(summary.CompletionTokens))
	dCachedCreationTokens := decimal.NewFromInt(int64(summary.CacheCreationTokens))
	dCompletionRatio := decimal.NewFromFloat(summary.CompletionRatio)
	dCacheRatio := decimal.NewFromFloat(summary.CacheRatio)
	dImageRatio := decimal.NewFromFloat(summary.ImageRatio)
	dModelRatio := decimal.NewFromFloat(summary.ModelRatio)
	dGroupRatio := decimal.NewFromFloat(summary.GroupRatio)
	dModelPrice := decimal.NewFromFloat(summary.ModelPrice)
	dCacheCreationRatio := decimal.NewFromFloat(summary.CacheCreationRatio)
	dCacheCreationRatio5m := decimal.NewFromFloat(summary.CacheCreationRatio5m)
	dCacheCreationRatio1h := decimal.NewFromFloat(summary.CacheCreationRatio1h)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio)
	summary.ToolCallSurchargeQuota = calculateTextToolCallSurcharge(ctx, relayInfo, &summary)

	var audioInputQuota decimal.Decimal
	if !relayInfo.PriceData.UsePrice {
		baseTokens := dPromptTokens

		var cachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			if !summary.IsClaudeUsageSemantic && !legacyClaudeDerived {
				baseTokens = baseTokens.Sub(dCacheTokens)
			}
			cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
		}

		var cachedCreationTokensWithRatio decimal.Decimal
		hasSplitCacheCreationTokens := summary.CacheCreationTokens5m > 0 || summary.CacheCreationTokens1h > 0
		if !dCachedCreationTokens.IsZero() || hasSplitCacheCreationTokens {
			if !summary.IsClaudeUsageSemantic && !legacyClaudeDerived {
				baseTokens = baseTokens.Sub(dCachedCreationTokens)
				cachedCreationTokensWithRatio = dCachedCreationTokens.Mul(dCacheCreationRatio)
			} else {
				remaining := max(summary.CacheCreationTokens-summary.CacheCreationTokens5m-summary.CacheCreationTokens1h, 0)
				cachedCreationTokensWithRatio = decimal.NewFromInt(int64(remaining)).Mul(dCacheCreationRatio)
				cachedCreationTokensWithRatio = cachedCreationTokensWithRatio.Add(decimal.NewFromInt(int64(summary.CacheCreationTokens5m)).Mul(dCacheCreationRatio5m))
				cachedCreationTokensWithRatio = cachedCreationTokensWithRatio.Add(decimal.NewFromInt(int64(summary.CacheCreationTokens1h)).Mul(dCacheCreationRatio1h))
			}
		}

		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			baseTokens = baseTokens.Sub(dImageTokens)
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		if !dAudioTokens.IsZero() {
			summary.AudioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(summary.ModelName)
			if summary.AudioInputPrice > 0 {
				baseTokens = baseTokens.Sub(dAudioTokens)
				audioInputQuota = decimal.NewFromFloat(summary.AudioInputPrice).
					Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			}
		}

		// OpenAI cache-write usage reports unadjusted prefix counts, so
		// cached_tokens + cache_write_tokens can exceed prompt_tokens and the
		// remainder can go negative. Clamp at zero so overlap never turns into
		// a negative base charge.
		if baseTokens.IsNegative() {
			baseTokens = decimal.Zero
		}

		promptQuota := baseTokens.Add(cachedTokensWithRatio).Add(imageTokensWithRatio).Add(cachedCreationTokensWithRatio)
		completionQuota := dCompletionTokens.Mul(dCompletionRatio)
		quotaCalculateDecimal := promptQuota.Add(completionQuota).Mul(ratio)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)
		quotaCalculateDecimal = relayInfo.PriceData.ApplyOtherRatiosToDecimal(quotaCalculateDecimal)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(summary.ToolCallSurchargeQuota)

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
		quota, clamp := common.QuotaFromDecimalChecked(quotaCalculateDecimal)
		summary.Quota = quota
		noteQuotaClamp(relayInfo, clamp)
	} else {
		quotaCalculateDecimal := dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)
		quotaCalculateDecimal = relayInfo.PriceData.ApplyOtherRatiosToDecimal(quotaCalculateDecimal)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(summary.ToolCallSurchargeQuota)
		quota, clamp := common.QuotaFromDecimalChecked(quotaCalculateDecimal)
		summary.Quota = quota
		noteQuotaClamp(relayInfo, clamp)
	}

	if !summary.hasBillableUsage() {
		summary.Quota = 0
	} else if !ratio.IsZero() && summary.Quota == 0 {
		summary.Quota = 1
	}

	return summary
}

func usageSemanticFromUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) string {
	if usage != nil && usage.UsageSemantic != "" {
		return usage.UsageSemantic
	}
	if relayInfo != nil && relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return "anthropic"
	}
	return "openai"
}

func PostTextConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent []string) {
	originUsage := usage
	billingUsage := effectiveBillingUsage(usage)
	if usage == nil {
		extraContent = append(extraContent, "上游无计费信息")
	}
	if originUsage != nil {
		ObserveChannelAffinityUsageCacheByRelayFormat(ctx, billingUsage, relayInfo.GetFinalRequestRelayFormat())
	}

	adminRejectReason := common.GetContextKeyString(ctx, constant.ContextKeyAdminRejectReason)
	summary := calculateTextQuotaSummary(ctx, relayInfo, billingUsage)

	var tieredResult *billingexpr.TieredResult
	tieredBillingApplied := false
	snap := relayInfo.TieredBillingSnapshot
	// Providers normally estimate missing usage before settlement. Preserve the
	// same prompt estimate when a fixed-price expression reaches us without it;
	// its conditions must still run and may select a token-priced fallback.
	if billingUsage == nil && snap != nil && billingexpr.UsesFixedPricing(snap.ExprString) {
		billingUsage = &dto.Usage{PromptTokens: summary.PromptTokens, CompletionTokens: summary.CompletionTokens, TotalTokens: summary.TotalTokens}
	}
	if billingUsage != nil {
		var tieredUsedVars map[string]bool
		if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
		}
		tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(billingUsage, summary.IsClaudeUsageSemantic, tieredUsedVars))
		if tieredOk {
			tieredBillingApplied = true
			tieredResult = tieredRes
			summary.Quota = composeTieredTextQuota(relayInfo, summary, tieredQuota, tieredRes)
			summary.FixedPriceBilling = isFixedPriceSettlement(relayInfo, tieredRes)
			if summary.FixedPriceBilling {
				summary.AudioInputPrice = 0
			}
			// B5-1 解释性日志：tiered 结算结果回填进 summary，供费用解释卡
			// 生成 tier_matched / fixed_price 事实与「命中阶梯价」推断。
			if tieredRes != nil {
				summary.MatchedTier = tieredRes.MatchedTier
				summary.BillingUnit = string(tieredRes.BillingUnit)
				if tieredRes.FixedPrice != nil {
					summary.FixedPriceValue = *tieredRes.FixedPrice
					summary.FixedPriceTier = tieredRes.MatchedTier
				}
			} else if snap := relayInfo.TieredBillingSnapshot; snap != nil {
				summary.MatchedTier = snap.EstimatedTier
				summary.BillingUnit = string(snap.EstimatedBillingUnit)
			}
		}
	}

	for _, item := range summary.ToolSurchargeItems {
		q := decimal.NewFromFloat(item.Price).
			Mul(decimal.NewFromInt(int64(item.Count))).
			Div(decimal.NewFromInt(1000)).
			Mul(decimal.NewFromFloat(summary.GroupRatio)).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		extraContent = append(extraContent, fmt.Sprintf(
			"%s 调用 %d 次，调用花费 %s",
			item.Name,
			item.Count,
			logger.LogQuota(common.QuotaFromDecimal(q)),
		))
	}
	if summary.AudioInputPrice > 0 && summary.AudioTokens > 0 {
		q := decimal.NewFromFloat(summary.AudioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(decimal.NewFromInt(int64(summary.AudioTokens))).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		extraContent = append(extraContent, fmt.Sprintf("Audio Input 花费 %s", logger.LogQuota(common.QuotaFromDecimal(q))))
	}

	if !summary.hasBillableUsage() {
		extraContent = append(extraContent, "上游没有返回计费信息，无法扣费（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, summary.ModelName, relayInfo.FinalPreConsumedQuota))
		// [修复] 空输出（HTTP 200 但 0 token、无计费信息）此前只写「消耗日志(type=2)」，
		// 错误日志(type=5)里一条都看不到 —— 管理端「错误日志」筛选因此毫无内容，
		// 排障（尤其是「首字很久 + ¥0 空回复」）无从下手。这里**额外**记一条错误日志，
		// 让这类「空壳成功响应」出现在错误日志里可直接筛选。仅在上游空流/无 usage 且
		// 非客户端主动断开（client_gone 是用户行为，不算上游故障）时记录。
		recordEmptyUpstreamResponse(ctx, relayInfo, summary)
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, summary.Quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, summary.Quota)
	}

	if err := SettleBilling(ctx, relayInfo, summary.Quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := summary.ModelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		extraContent = append(extraContent, fmt.Sprintf("模型 %s", summary.ModelName))
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		extraContent = append(extraContent, fmt.Sprintf("模型 %s", summary.ModelName))
	}

	logContent := strings.Join(extraContent, ", ")
	var other *model.LogOther
	if summary.IsClaudeUsageSemantic {
		other = GenerateClaudeOtherInfo(ctx, relayInfo,
			summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio,
			summary.CacheTokens, summary.CacheRatio,
			summary.CacheCreationTokens, summary.CacheCreationRatio,
			summary.CacheCreationTokens5m, summary.CacheCreationRatio5m,
			summary.CacheCreationTokens1h, summary.CacheCreationRatio1h,
			summary.ModelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
		other.SetPublic("usage_semantic", "anthropic")
	} else {
		other = GenerateTextOtherInfo(ctx, relayInfo, summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio, summary.CacheTokens, summary.CacheRatio, summary.ModelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	}
	appendUsageBillingPathForLog(other, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens), originUsage)
	// B5-1 解释性日志：facts 来自现有计费数据（零额外计算），结构化写入
	// other.explain 供前端"费用解释"卡片渲染。
	appendBillingExplain(ctx, other, summary)
	if adminRejectReason != "" {
		other.SetAdmin("reject_reason", adminRejectReason)
	}
	if summary.ImageTokens != 0 {
		other.SetPublic("image", true)
		other.SetPublic("image_ratio", summary.ImageRatio)
		other.SetPublic("image_output", summary.ImageTokens)
	}
	appendToolSurchargeLogInfo(other, summary.ToolSurchargeItems)
	if summary.AudioInputPrice > 0 && summary.AudioTokens > 0 {
		other.SetPublic("audio_input_seperate_price", true)
		other.SetPublic("audio_input_token_count", summary.AudioTokens)
		other.SetPublic("audio_input_price", summary.AudioInputPrice)
	}
	if summary.CacheCreationTokens > 0 {
		other.SetPublic("cache_creation_tokens", summary.CacheCreationTokens)
		other.SetPublic("cache_creation_ratio", summary.CacheCreationRatio)
	}
	if summary.CacheCreationTokens5m > 0 {
		other.SetPublic("cache_creation_tokens_5m", summary.CacheCreationTokens5m)
		other.SetPublic("cache_creation_ratio_5m", summary.CacheCreationRatio5m)
	}
	if summary.CacheCreationTokens1h > 0 {
		other.SetPublic("cache_creation_tokens_1h", summary.CacheCreationTokens1h)
		other.SetPublic("cache_creation_ratio_1h", summary.CacheCreationRatio1h)
	}
	cacheWriteTokens := cacheWriteTokensTotal(summary)
	if cacheWriteTokens > 0 {
		// cache_write_tokens: normalized cache creation total for UI display.
		// If split 5m/1h values are present, this is their sum; otherwise it falls back
		// to cache_creation_tokens.
		other.SetPublic("cache_write_tokens", cacheWriteTokens)
	}
	if relayInfo.GetFinalRequestRelayFormat() != types.RelayFormatClaude && billingUsage != nil && billingUsage.UsageSource != "" && billingUsage.InputTokens > 0 {
		// input_tokens_total: explicit normalized total input used by the usage log UI.
		// Only write this field when upstream/current conversion has already provided a
		// reliable total input value and tagged the usage source. Do not infer it from
		// prompt/cache fields here, otherwise old upstream payloads may be double-counted.
		other.SetPublic("input_tokens_total", billingUsage.InputTokens)
	}
	if tieredBillingApplied {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}

	attachQuotaSaturation(ctx, relayInfo, other)

	// §4.7.1：记录用户最近一次成功调用的模型（轻量记忆，供前端回填默认模型）。
	RecordUserLastModel(relayInfo.UserId, relayInfo.OriginModelName)

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     summary.PromptTokens,
		CompletionTokens: summary.CompletionTokens,
		ModelName:        logModel,
		TokenName:        summary.TokenName,
		Quota:            summary.Quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(summary.UseTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		RequestBytes:     relayInfo.RequestBytes,
		ResponseBytes:    relayInfo.ResponseBytes,
	})
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(summary.CompletionTokens))
	})
}

// emptyUpstreamResponseIsError 判定「空壳响应」是否应记入错误日志。
// 仅当上游以非正常原因结束（eof/timeout/scanner_error/panic/ping_fail）且**不是**
// 客户端主动断开（client_gone——用户取消/切页/网络抖动，非上游故障）时，才视为
// 需要排障的空输出。无 StreamStatus（非流式）时也记（非流式返回 0 token 同样异常）。
func emptyUpstreamResponseIsError(relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil {
		return true
	}
	ss := relayInfo.StreamStatus
	if ss == nil {
		// 非流式：0 token 且无计费信息本身就是异常（上游可能超时/空体）。
		return true
	}
	switch ss.EndReason {
	case relaycommon.StreamEndReasonClientGone:
		return false
	case relaycommon.StreamEndReasonNone:
		// 流结束但从未设置原因（异常/未走完状态机）：与非流式 0 token 一样视为异常。
		return true
	case relaycommon.StreamEndReasonDone:
		// 正常 done 但 0 token：上游确实回了空内容——仍值得记（用户看到空回复）。
		return true
	default:
		// eof / timeout / scanner_error / panic / ping_fail 等：异常结束。
		return true
	}
}

// recordEmptyUpstreamResponse 为「HTTP 200 但 0 token、无计费信息」的空输出记录一条
// 错误日志（type=5），使这类问题在管理端「错误日志」视图可见、可筛选。
// 客户端主动断开的（client_gone）不记（用户行为，非上游故障）。
func recordEmptyUpstreamResponse(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, summary textQuotaSummary) {
	if relayInfo == nil || ctx == nil {
		return
	}
	if !emptyUpstreamResponseIsError(relayInfo) {
		return
	}
	other := model.NewLogOther()
	other.SetPublic("error_type", "empty_response")
	other.SetPublic("error_code", "upstream_empty_response")
	other.SetPublic("status_code", 200)
	other.SetPublic("error_class", "server_error")
	other.SetPublic("empty_response", true)
	if relayInfo.StreamStatus != nil {
		other.SetPublic("end_reason", string(relayInfo.StreamStatus.EndReason))
	}
	// 首字耗时（若有）——「首字很久 + ¥0」是本次排障的核心特征，务必带上。
	if !relayInfo.FirstResponseTime.IsZero() && !relayInfo.StartTime.IsZero() {
		other.SetPublic("frt", float64(relayInfo.FirstResponseTime.UnixMilli()-relayInfo.StartTime.UnixMilli()))
	}
	if relayInfo.RequestBytes > 0 {
		other.SetPublic("request_bytes", relayInfo.RequestBytes)
	}
	if relayInfo.ResponseBytes > 0 {
		other.SetPublic("response_bytes", relayInfo.ResponseBytes)
	}
	other.SetAdmin("use_channel", ctx.GetStringSlice("use_channel"))
	if relayInfo.ChannelId > 0 {
		other.SetAdmin("channel_id", relayInfo.ChannelId)
	}
	content := "上游返回空响应（HTTP 200 但无 token / 无计费信息）"
	useTimeSeconds := int(summary.UseTimeSeconds)
	model.RecordErrorLog(ctx, relayInfo.UserId, relayInfo.ChannelId, relayInfo.OriginModelName, summary.TokenName, content, relayInfo.TokenId, useTimeSeconds, relayInfo.IsStream, relayInfo.UsingGroup, other)
}

