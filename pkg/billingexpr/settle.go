package billingexpr

import (
	"fmt"

	"github.com/lza6/new-api-Max/common"
)

// quotaConversion converts raw expression output to quota based on the
// expression version. This is the central dispatch point for future versions
// that may use a different conversion formula.
func quotaConversion(exprOutput float64, snap *BillingSnapshot) float64 {
	if snap.TaskUsageBilling {
		return exprOutput * snap.QuotaPerUnit
	}
	switch snap.ExprVersion {
	default: // v1: coefficients are $/1M tokens prices
		return exprOutput / 1_000_000 * snap.QuotaPerUnit
	}
}

// ComputeTieredQuota runs the Expr from a frozen BillingSnapshot against
// actual token counts and returns the settlement result.
func ComputeTieredQuota(snap *BillingSnapshot, params TokenParams) (TieredResult, error) {
	return ComputeTieredQuotaWithRequest(snap, params, RequestInput{})
}

func ComputeTieredQuotaWithRequest(snap *BillingSnapshot, params TokenParams, request RequestInput) (TieredResult, error) {
	if snap.TaskUsageBilling && UsesFixedPricing(snap.ExprString) {
		return TieredResult{}, fmt.Errorf("fixed pricing is not supported for task usage expressions")
	}
	cost, trace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, params, request)
	if err != nil {
		return TieredResult{}, err
	}

	// 负费用等于倒贴：表达式可能在某些输入组合下转负（典型是「缓存折扣」项
	// 超过本金，例如 p*3 + c*15 - cr*6 在缓存命中量很大时为负）。保存期的
	// SmokeTest 只跑 P/C/Len 向量（CR/CC/Img/AI 恒为 0），覆盖不到这类组合，
	// 所以运行时必须兜底。钳到 0 并留审计记录，绝不把负数送进结算——
	// 否则每命中一次缓存反而给用户返额度。
	if cost < 0 {
		common.SysError(fmt.Sprintf(
			"billingexpr: expression %q produced negative cost %.6f (p=%.0f c=%.0f len=%.0f cr=%.0f cc=%.0f img=%.0f ai=%.0f); clamped to 0",
			snap.ExprHash, cost, params.P, params.C, params.Len, params.CR, params.CC, params.Img, params.AI))
		cost = 0
	}

	quotaBeforeGroup := quotaConversion(cost, snap)
	afterGroup, clamp := common.QuotaRoundChecked(quotaBeforeGroup * snap.GroupRatio)
	crossed := trace.MatchedTier != snap.EstimatedTier

	return TieredResult{
		BillingUnit:            trace.BillingUnit,
		FixedPrice:             trace.FixedPrice,
		ActualQuotaBeforeGroup: quotaBeforeGroup,
		ActualQuotaAfterGroup:  afterGroup,
		MatchedTier:            trace.MatchedTier,
		RequestRules:           trace.RequestRules,
		CrossedTier:            crossed,
		Clamp:                  clamp,
	}, nil
}
