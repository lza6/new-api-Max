package model

import (
	"slices"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/dto"
)

// ChannelHealthProbe 由 service 层注册，返回渠道健康分快照。
// 用于 FilterChannelHealth：冷却剔除 + 低分剔除。nil = 不启用。
var ChannelHealthProbe func(channelID int) (score float64, coolingDown bool, hasSamples bool)

var filterEvalOrder = []dto.ChannelFilterKind{
	dto.FilterRequestPath,
	dto.FilterTaskPluginIdentity,
	dto.FilterChannelHealth,
}

// ChannelSatisfiesFilters reports whether ch passes every filter.
// On false, it returns the kind of the first violated filter (request_path
// then task_plugin_identity) for error attribution.
func ChannelSatisfiesFilters(ch *Channel, modelName string, filters []dto.ChannelFilter) (bool, dto.ChannelFilterKind) {
	if ch == nil {
		return false, ""
	}
	for _, kind := range filterEvalOrder {
		for _, filter := range filters {
			if filter.Kind != kind {
				continue
			}
			if !channelMatchesFilter(ch, modelName, filter) {
				return false, kind
			}
		}
	}
	return true, ""
}

// filterCandidateIDs applies filters to a cached candidate id list.
// Caller must hold channelSyncLock (read lock). The input slice is never mutated.
// A missing id in channelsIDM is kept for request_path (downstream consistency
// error) and dropped for task_plugin_identity, matching the previous filters.
func filterCandidateIDs(ids []int, modelName string, filters []dto.ChannelFilter) (kept []int, emptiedBy dto.ChannelFilterKind) {
	if len(ids) == 0 {
		return ids, ""
	}
	kept = ids
	for _, kind := range filterEvalOrder {
		kindFilters := filtersByKind(filters, kind)
		if len(kindFilters) == 0 {
			continue
		}
		next := make([]int, 0, len(kept))
		for _, id := range kept {
			channel, exists := channelsIDM[id]
			if candidatePassesKindFilters(channel, exists, modelName, kind, kindFilters) {
				next = append(next, id)
			}
		}
		if len(kept) > 0 && len(next) == 0 {
			return next, kind
		}
		kept = next
	}
	return kept, ""
}

func filtersByKind(filters []dto.ChannelFilter, kind dto.ChannelFilterKind) []dto.ChannelFilter {
	var matched []dto.ChannelFilter
	for _, filter := range filters {
		if filter.Kind == kind {
			matched = append(matched, filter)
		}
	}
	return matched
}

func candidatePassesKindFilters(ch *Channel, exists bool, modelName string, kind dto.ChannelFilterKind, filters []dto.ChannelFilter) bool {
	if kind == dto.FilterRequestPath && !exists {
		return true
	}
	if !exists || ch == nil {
		return false
	}
	for _, filter := range filters {
		if !channelMatchesFilter(ch, modelName, filter) {
			return false
		}
	}
	return true
}

func channelMatchesFilter(ch *Channel, modelName string, filter dto.ChannelFilter) bool {
	switch filter.Kind {
	case dto.FilterRequestPath:
		if filter.RequestPath == "" {
			return true
		}
		if ch.Type != constant.ChannelTypeAdvancedCustom {
			return true
		}
		config := ch.GetOtherSettings().AdvancedCustom
		return config != nil && config.SupportsPathForModel(filter.RequestPath, modelName)
	case dto.FilterTaskPluginIdentity:
		if ch.Type == constant.ChannelTypeTaskPlugin {
			return filter.TaskPluginKey != "" && ch.GetSetting().TaskPluginKey == filter.TaskPluginKey
		}
		return filter.TaskPluginKey == "" || slices.Contains(filter.TaskPluginChannelTypes, ch.Type)
	case dto.FilterChannelHealth:
		// T2-2 健康分路由：仅当启用全局开关时过滤。
		// 冷却中剔除（无论是否设分数阈值）；健康分低于阈值剔除（阈值 <=0 = 只做冷却剔除）。
		// fail-open：无回调/开关关闭时放行所有候选（行为与现状一致）。
		if !ChannelHealthRoutingEnabled() {
			return true
		}
		if ChannelHealthProbe == nil {
			return true
		}
		score, cooling, hasSamples := ChannelHealthProbe(ch.Id)
		if filter.HealthCoolingExclude && cooling {
			return false
		}
		if filter.HealthMinScore <= 0 {
			return true
		}
		// fail-open：无样本（新渠道/冷启动）不按分剔除，避免新渠道永远选不到。
		if !hasSamples {
			return true
		}
		return score >= float64(filter.HealthMinScore)
	default:
		return true
	}
}

// channelHealthRoutingEnabled 全局开关：CHANNEL_HEALTH_ROUTING=on|true 开启
// 健康分路由过滤（T2-2）。默认 on（keep 灰度通过后的默认行为），运维可
// 显式设 off 关闭（回滚路径）。零值默认需要热更，但为避免 model 层依赖
// config 热更系统，这里用 env 一次性读取（进程级快照，热更不覆盖 env）。
var channelHealthRoutingEnabled = common.GetEnvOrDefaultBool("CHANNEL_HEALTH_ROUTING", true)

// SetChannelHealthRoutingEnabled 测试用覆盖；传 nil 恢复 env 默认。
func SetChannelHealthRoutingEnabled(v *bool) {
	if v == nil {
		channelHealthRoutingEnabled = common.GetEnvOrDefaultBool("CHANNEL_HEALTH_ROUTING", true)
		return
	}
	channelHealthRoutingEnabled = *v
}

// ChannelHealthRoutingEnabled 报告健康分路由开关状态。
func ChannelHealthRoutingEnabled() bool {
	return channelHealthRoutingEnabled
}
