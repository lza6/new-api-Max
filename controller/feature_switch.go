package controller

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/feature_switch"
	"github.com/lza6/new-api-Max/setting/response_cache_setting"
)

// 能力开关（Feature Switch）管理端接口 —— Batch-8 / G1。
//
// 鉴权：路由上挂 `middleware.RootAuth()`（改的是全局能力，非 root 不可触及）。
// 审计：每次变更写 `feature_switch.update`，含 key / 变更前后值 / 风险级别 / 操作人；
// 审计内容**不含任何密钥**（开关值本身即 true|false|off|shadow|enforce，非敏感）。
//
// 一致性策略（重要）：
//  1. 先在内存中校验并应用（`feature_switch.Set`，失败即时返回，不落库）；
//  2. 再跑该开关的**启用前置动作**（如加密既有渠道密钥）；失败则把内存回滚到变更前；
//  3. 最后持久化。持久化失败时同样回滚内存**并把旧值重新落库**，避免
//     「内存已改、库里没改」在重启后产生行为落差。

// featureSwitchOptionKey 是持久化选项键（`模块名.字段`，config 框架的约定格式）。
const featureSwitchOptionKey = feature_switch.ModuleName + "." + feature_switch.ValuesFieldKey

// GetFeatureSwitches 返回全部能力开关的当前状态与效果度量。
//
// 度量口径：只返回**能廉价算出的进程内计数/聚合**，与 `/metrics` 同名指标同源。
// 未接入指标的开关在 `metrics` 里没有对应键，前端据此显示「该开关暂无指标」——
// 不编造数字。
func GetFeatureSwitches(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"switches":         feature_switch.List(),
		"metrics":          featureSwitchMetrics(),
		"metrics_endpoint": "/metrics",
	})
}

type featureSwitchUpdateRequest struct {
	// Key 是开关的 env 名（稳定契约）。
	Key string `json:"key" binding:"required"`
	// Value 是目标值；Reset 为 true 时可省略。
	Value string `json:"value"`
	// Reset 为 true 时清除管理员配置，回退 env 默认。
	Reset bool `json:"reset"`
}

// UpdateFeatureSwitch 变更单个能力开关。
func UpdateFeatureSwitch(c *gin.Context) {
	var req featureSwitchUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request body")
		return
	}

	meta, ok := feature_switch.Meta(req.Key)
	if !ok {
		common.ApiErrorMsg(c, "unknown feature switch: "+req.Key)
		return
	}
	if !meta.AdminEditable {
		common.ApiErrorMsg(c, "feature switch is not editable from the admin console: "+req.Key)
		return
	}

	before, _ := feature_switch.CurrentValue(req.Key)
	previousConfigured, hadPrevious := feature_switch.ConfiguredValue(req.Key)

	if req.Reset {
		if err := feature_switch.Reset(req.Key); err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		if req.Value == "" {
			common.ApiErrorMsg(c, "value is required unless reset is true")
			return
		}
		if err := feature_switch.Set(req.Key, req.Value); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	// 启用前置动作（如把既有明文渠道密钥加密回写）。失败必须回滚，否则会留下
	// 「开关已开但数据没迁移」的危险中间态。
	if !req.Reset {
		if err := activateFeatureSwitch(req.Key, req.Value); err != nil {
			restoreFeatureSwitch(req.Key, previousConfigured, hadPrevious)
			common.ApiError(c, err)
			return
		}
	}

	encoded, err := feature_switch.SerializedValues()
	if err != nil {
		restoreFeatureSwitch(req.Key, previousConfigured, hadPrevious)
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOption(featureSwitchOptionKey, encoded); err != nil {
		restoreFeatureSwitch(req.Key, previousConfigured, hadPrevious)
		if persistErr := persistFeatureSwitches(); persistErr != nil {
			common.SysError("feature switch rollback persist failed: " + persistErr.Error())
		}
		common.ApiError(c, err)
		return
	}

	after, _ := feature_switch.CurrentValue(req.Key)
	recordManageAudit(c, "feature_switch.update", map[string]any{
		"key":     req.Key,
		"from":    before,
		"to":      after,
		"reset":   req.Reset,
		"risk":    string(meta.Risk),
		"success": true,
	})

	common.ApiSuccess(c, gin.H{
		"switches": feature_switch.List(),
		"metrics":  featureSwitchMetrics(),
	})
}

// activateFeatureSwitch 执行某些开关「打开时」必须配套完成的数据动作。
//
// 调用时机：内存值**已经**被设置之后（迁移/初始化函数据此判断是否需要动作），
// 返回错误时调用方负责回滚内存值。
func activateFeatureSwitch(key, value string) error {
	if value != "true" {
		return nil
	}
	switch key {
	case common.FlagChannelKeyEncryption:
		// 打开加密后，库里已有的**明文**渠道 key 必须就地加密回写；否则
		// 读侧会按密文处理明文，全部渠道鉴权失败。该函数幂等（跳过已加密行）。
		if err := model.MigrateChannelKeyEncryption(); err != nil {
			return fmt.Errorf("encrypt existing channel keys before enabling: %w", err)
		}
		model.AssertChannelKeyEncryptionState()
	case common.FlagPasswordLoginEncryption:
		// 登录加密依赖一把持久化的非对称私钥；未初始化就打开会让登录直接失败。
		// 该函数幂等（已有 key 则只加载）。
		if err := model.InitPasswordEncryption(); err != nil {
			return fmt.Errorf("initialize login encryption key before enabling: %w", err)
		}
	}
	return nil
}

// restoreFeatureSwitch 把某个开关的内存状态恢复到变更前。
func restoreFeatureSwitch(key, previousConfigured string, hadPrevious bool) {
	if hadPrevious {
		if err := feature_switch.Set(key, previousConfigured); err != nil {
			common.SysError("feature switch rollback failed for " + key + ": " + err.Error())
		}
		return
	}
	if err := feature_switch.Reset(key); err != nil {
		common.SysError("feature switch rollback reset failed for " + key + ": " + err.Error())
	}
}

// persistFeatureSwitches 把当前全部开关配置写回选项表（用于变更失败后的补偿落库）。
func persistFeatureSwitches() error {
	encoded, err := feature_switch.SerializedValues()
	if err != nil {
		return err
	}
	return model.UpdateOption(featureSwitchOptionKey, encoded)
}

// featureSwitchMetrics 汇总各开关的**效果度量**（与 /metrics 同名指标同源）。
//
// 只做进程内读数与一次渠道 id 扫描（与 /metrics 渲染同量级），不做重查询。
// DB 未就绪 / 查询失败时不编造数字，直接省略对应键。
func featureSwitchMetrics() map[string]float64 {
	out := map[string]float64{}

	for k, v := range ResponseCacheMetrics() {
		out[k] = v
	}
	for k, v := range ToolDrawerMetrics() {
		out[k] = v
	}

	var auditFindings int64
	for _, n := range service.RelayAuditSnapshot() {
		auditFindings += n
	}
	out["relay_audit_findings_total"] = float64(auditFindings)

	out["policy_engine_eval_total"] = float64(service.PolicyEvalTotal())
	var policyDecisions int64
	for _, n := range service.PolicySnapshot() {
		policyDecisions += n
	}
	out["policy_decision_total"] = float64(policyDecisions)

	if model.DB == nil {
		return out
	}
	var channelIds []int
	if err := model.DB.Model(&model.Channel{}).Pluck("id", &channelIds).Error; err != nil {
		return out
	}
	var openCircuits, tracked int
	var scoreSum float64
	for _, id := range channelIds {
		if state, _ := service.GetChannelCircuitState(id); state == service.CircuitOpen {
			openCircuits++
		}
		scoreSum += service.GetChannelHealthSnapshot(id).Score
		tracked++
	}
	out["channel_circuit_open_total"] = float64(openCircuits)
	out["channels_tracked"] = float64(tracked)
	// 恒定产出（无渠道时为 0）：注册表声明了这个指标，就必须始终存在 ——
	// 否则「实验功能」页在有/无渠道两种状态下表现不一致，且守门用例会判为声明与实现不符。
	out["channel_health_score_avg"] = 0
	if tracked > 0 {
		out["channel_health_score_avg"] = scoreSum / float64(tracked)
	}
	return out
}

// responseCacheMetricKeys 是响应缓存开关在注册表里声明的指标名。
// 单独列出并加一条**断言**：声明了就必须有实现，否则「实验功能」页会永远显示
// 「暂无数据」——那正是本项目最忌讳的伪闭环。
var responseCacheMetricKeys = []string{
	"response_cache_hits_total",
	"response_cache_misses_total",
	"response_cache_live_entries",
}

// ResponseCacheMetrics 返回响应缓存的效果度量（与 /metrics 同名指标同源）。
func ResponseCacheMetrics() map[string]float64 {
	stats := response_cache_setting.Stats()
	return map[string]float64{
		"response_cache_hits_total":   float64(stats.Hits),
		"response_cache_misses_total": float64(stats.Misses),
		"response_cache_live_entries": float64(service.ResponseCacheLiveEntries()),
	}
}

// ToolDrawerMetrics 返回工具抽屉的效果度量（去重次数与被移除的字节量）。
func ToolDrawerMetrics() map[string]float64 {
	savings := service.ToolDrawerSavings()
	return map[string]float64{
		"tool_drawer_deduped_requests_total": float64(savings.DedupedRequests),
		"tool_drawer_saved_bytes_total":      float64(savings.SavedBytes),
	}
}
