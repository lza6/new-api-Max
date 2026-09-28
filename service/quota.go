package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/pkg/billingexpr"
	perfmetrics "github.com/lza6/new-api-Max/pkg/perf_metrics"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/setting/ratio_setting"
	"github.com/lza6/new-api-Max/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) (int, *common.QuotaClamp) {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return common.QuotaFromDecimalChecked(quota)
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return common.QuotaFromDecimalChecked(quota)
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return err
	}

	token, err := model.GetTokenByKey(strings.TrimPrefix(relayInfo.TokenKey, "sk-"), false)
	if err != nil {
		return err
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	groupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)

	autoGroup, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroup)
	if exists {
		groupRatio = ratio_setting.GetGroupRatio(autoGroup.(string))
		logger.LogDebug(ctx, "final group ratio: %f", groupRatio)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	actualGroupRatio := groupRatio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		actualGroupRatio = userGroupRatio
	}

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   relayInfo.UsePrice,
		ModelRatio: modelRatio,
		GroupRatio: actualGroupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)

	if userQuota < quota {
		return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota))
	}

	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}

	err = PostConsumeQuota(relayInfo, quota, 0, false)
	if err != nil {
		return err
	}
	logger.LogInfo(ctx, "realtime streaming consume quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) {

	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, billingexpr.TokenParams{
		P:   float64(usage.InputTokens),
		C:   float64(usage.OutputTokens),
		Len: float64(usage.InputTokens),
	})
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		RequestBytes:     relayInfo.RequestBytes,
		ResponseBytes:    relayInfo.ResponseBytes,
	})
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	value := (cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)
	quota, clamp := common.QuotaRoundChecked(value)
	if clamp != nil {
		return -1
	}
	return quota
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {
	if usage == nil {
		usage = &dto.Usage{PromptTokens: relayInfo.GetEstimatePromptTokens(), TotalTokens: relayInfo.GetEstimatePromptTokens()}
	}

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}
	fixedPriceBilling := tieredOk && isFixedPriceSettlement(relayInfo, tieredRes)

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	billingModelName := relayInfo.GetBillingModelName()
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(billingModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(billingModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(billingModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  billingModelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 && !fixedPriceBilling {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, billingModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := billingModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		RequestBytes:     relayInfo.RequestBytes,
		ResponseBytes:    relayInfo.ResponseBytes,
	})
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(usage.CompletionTokens))
	})
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	// 原子预扣：检查与扣减在同一操作中完成，并发请求不可能同时通过检查后超扣。
	reserved, err := model.TryReserveTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota, relayInfo.TokenUnlimited)
	if err != nil {
		return err
	}
	if !reserved {
		remainQuota := 0
		if token, tokenErr := model.GetTokenByKey(relayInfo.TokenKey, false); tokenErr == nil && token != nil {
			remainQuota = token.RemainQuota
		}
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(remainQuota), logger.FormatQuota(quota))
	}
	return nil
}

type postConsumeQuotaResult struct {
	FundingApplied bool
	TokenApplied   bool
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) error {
	_, err := postConsumeQuotaWithResult(relayInfo, quota, preConsumedQuota, sendEmail)
	return err
}

func postConsumeQuotaWithResult(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (result postConsumeQuotaResult, err error) {

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			return result, errors.New("subscription id is missing")
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				return result, err
			}
			relayInfo.SubscriptionPostDelta += delta
		}
	} else {
		// Wallet
		if quota > 0 {
			err = model.DecreaseUserQuota(relayInfo.UserId, quota, false)
		} else {
			err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
		}
		if err != nil {
			return result, err
		}
	}
	result.FundingApplied = true

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return result, err
		}
		result.TokenApplied = true
	}

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return result, nil
}

// quotaWarnUpdateLocks 串行化同一用户的 UserSetting 读-改-写：多个并发额度预警
// goroutine 若同时基于同一旧快照写回 QuotaWarnedLevels，会重复触发同一档位。
// 分片锁内存有界、无需清理；用户侧主动修改设置走同样的 UpdateUserSetting，与
// billing_session 的既有读写模式保持一致。
var quotaWarnUpdateLocks [256]sync.Mutex

func lockQuotaWarnUpdate(userId int) func() {
	mu := &quotaWarnUpdateLocks[uint(userId)%uint(len(quotaWarnUpdateLocks))]
	mu.Lock()
	return mu.Unlock
}

// decideQuotaWarn 判定当前剩余额度需要提醒的档位（纯函数，便于单测）。
// explicitThreshold != 0：用户显式单档，最高优先级，不记录档位（防重复交给
// CheckNotificationLimit），保持存量行为。否则按 thresholds 多档：remaining 低于
// 某档且该档未在 alreadyWarned 记录过 → 触发；已记录档位在 remaining 回升到该档
// 以上后剔除（充值/额度增长后下次再降到该档可再次提醒）。
// 返回 (本次需提醒的档位, 更新后应持久化的已记录档位)；thresholds 需为从大到小。
func decideQuotaWarn(remaining int64, explicitThreshold float64, thresholds []int, alreadyWarned []int) (toNotify []int, newWarned []int) {
	if explicitThreshold != 0 {
		if remaining < int64(int(explicitThreshold)) {
			return []int{int(explicitThreshold)}, alreadyWarned
		}
		return nil, alreadyWarned
	}

	// 先剔除已回升到该档以上的记录，实现充值后降档重置。
	for _, warned := range alreadyWarned {
		if remaining < int64(warned) {
			newWarned = append(newWarned, warned)
		}
	}
	// thresholds 已按从大到小排序，toNotify 保持该触发顺序。
	for _, tier := range thresholds {
		if remaining >= int64(tier) {
			continue
		}
		if !slices.Contains(newWarned, tier) {
			toNotify = append(toNotify, tier)
			newWarned = append(newWarned, tier)
		}
	}
	return toNotify, newWarned
}

// usingExplicitQuotaWarn 判定用户额度预警是否走「显式单档」最高优先级路径。
// 仅当阈值非零且非注册注入默认（QuotaWarnThresholdsDefault=false）时为显式；
// 注册注入的默认 80% 阈值（B6-3）标记为 default=true，走多档默认（P1-1 审查修复）。
func usingExplicitQuotaWarn(userSetting dto.UserSetting) bool {
	return userSetting.QuotaWarningThreshold != 0 && !userSetting.QuotaWarnThresholdsDefault
}

// refreshQuotaWarnLevels 在并发安全的读-改-写下读取并持久化用户已触发档位，
// 返回本次需提醒的档位。仅当档位集合实际变化时才写回 setting JSON 列，避免
// 每个完成请求无谓写库。
func refreshQuotaWarnLevels(relayInfo *relaycommon.RelayInfo, remaining int64) []int {
	unlock := lockQuotaWarnUpdate(relayInfo.UserId)
	defer unlock()

	currentSetting, err := model.GetUserSetting(relayInfo.UserId, false)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to load user setting for quota warn levels: %s", err.Error()))
		currentSetting = relayInfo.UserSetting
	}
	toNotify, newWarned := decideQuotaWarn(remaining, 0, common.QuotaWarnThresholds, currentSetting.QuotaWarnedLevels)
	if !slices.Equal(newWarned, currentSetting.QuotaWarnedLevels) {
		updated := currentSetting
		updated.QuotaWarnedLevels = newWarned
		if err := model.UpdateUserSetting(relayInfo.UserId, updated); err != nil {
			common.SysError(fmt.Sprintf("failed to persist quota warn levels for user %d: %s", relayInfo.UserId, err.Error()))
		}
	}
	return toNotify
}

// buildQuotaWarnNotify 根据通知渠道构建额度预警消息（Email/Webhook 支持 HTML，
// Bark/Gotify 使用简短纯文本）。
func buildQuotaWarnNotify(userSetting dto.UserSetting, prompt string, displayQuota int) dto.Notify {
	topUpLink := PaymentReturnURL("/wallet")
	notifyType := userSetting.NotifyType
	if notifyType == "" {
		notifyType = dto.NotifyTypeEmail
	}
	var content string
	var values []any
	switch notifyType {
	case dto.NotifyTypeBark:
		content = "{{value}}，剩余额度：{{value}}，请及时充值"
		values = []any{prompt, logger.FormatQuota(displayQuota)}
	case dto.NotifyTypeGotify:
		content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
		values = []any{prompt, logger.FormatQuota(displayQuota)}
	default:
		content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
		values = []any{prompt, logger.FormatQuota(displayQuota), topUpLink, topUpLink}
	}
	return dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)
}

// sendQuotaWarnNotify 按用户通知渠道发送额度预警，失败仅记日志，不影响计费主链路。
func sendQuotaWarnNotify(relayInfo *relaycommon.RelayInfo, userSetting dto.UserSetting, prompt string, displayQuota int) {
	notify := buildQuotaWarnNotify(userSetting, prompt, displayQuota)
	if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, userSetting, notify); err != nil {
		common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
	}
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		if relayInfo == nil || relayInfo.UserId == 0 {
			return
		}
		userSetting := relayInfo.UserSetting
		consumeQuota := int64(quota) + int64(preConsumedQuota)
		remaining := int64(relayInfo.UserQuota) - consumeQuota

		// 用户显式单档：最高优先级，保持存量行为（不记录档位，防重复交给限频）。
		// QuotaWarnThresholdsDefault=true 表示阈值是注册注入的默认值（B6-3 的 80%），
		// 非"用户显式设置"——这类用户走多档默认，避免多档对主流用户不可达（审查 P1-1）。
		if usingExplicitQuotaWarn(userSetting) {
			toNotify, _ := decideQuotaWarn(remaining, userSetting.QuotaWarningThreshold, nil, nil)
			if len(toNotify) == 0 {
				return
			}
			prompt := "您的额度即将用尽"
			sendQuotaWarnNotify(relayInfo, userSetting, prompt, relayInfo.UserQuota)
			return
		}

		// 多档分级提醒：每个新触发的档位提醒一次；充值/额度增长后档位重置可再提醒。
		toNotify := refreshQuotaWarnLevels(relayInfo, remaining)
		for _, tier := range toNotify {
			prompt := fmt.Sprintf("您的额度即将用尽（剩余额度低于 %s）", logger.FormatQuota(tier))
			sendQuotaWarnNotify(relayInfo, userSetting, prompt, relayInfo.UserQuota)
		}
	})
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	gopool.Go(func() {
		if relayInfo == nil || relayInfo.UserId == 0 {
			return
		}
		if relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
			return
		}

		userSetting := relayInfo.UserSetting
		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter

		// 用户显式单档：最高优先级，保持存量行为（不记录档位，防重复交给限频）。
		// QuotaWarnThresholdsDefault=true 表示默认注入阈值（走多档），见钱包路径注释（审查 P1-1）。
		if usingExplicitQuotaWarn(userSetting) {
			toNotify, _ := decideQuotaWarn(remaining, userSetting.QuotaWarningThreshold, nil, nil)
			if len(toNotify) == 0 {
				return
			}
			prompt := "您的订阅额度即将用尽"
			sendQuotaWarnNotify(relayInfo, userSetting, prompt, int(remaining))
			return
		}

		// 多档分级提醒：复用 refreshQuotaWarnLevels 的档位台账（与钱包额度同源）。
		toNotify := refreshQuotaWarnLevels(relayInfo, remaining)
		for _, tier := range toNotify {
			prompt := fmt.Sprintf("您的订阅额度即将用尽（剩余额度低于 %s）", logger.FormatQuota(tier))
			sendQuotaWarnNotify(relayInfo, userSetting, prompt, int(remaining))
		}
	})
}
