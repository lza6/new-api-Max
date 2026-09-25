package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/dto"
	"github.com/lza6/new-api-Max/logger"
	kitdto "github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*kitdto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

// channelRuntimeSnapshots 缓存渠道“静态可预计算”的运行时配置（设置/其他设置/
// 参数覆盖/头覆盖/模型映射/状态码映射/自动封禁/baseURL），避免热路径每请求
// 重复 JSON 解析与 map 分配。动态字段（多 key 轮询索引、启用状态）仍读
// channelsIDM/Channel.ChannelInfo，不入快照。全量 InitChannelCache 重建；
// CacheUpdateChannelStatus 只改 Status 不动快照（快照不含状态）。
var channelRuntimeSnapshots map[int]*ChannelRuntimeSnapshot

// ChannelRuntimeSnapshot 是渠道静态运行时配置的预计算只读快照。
type ChannelRuntimeSnapshot struct {
	Settings          kitdto.ChannelSettings
	OtherSettings     kitdto.ChannelOtherSettings
	ParamOverride     map[string]any
	HeaderOverride    map[string]any
	ModelMapping      string
	StatusCodeMapping string
	AutoBan           bool
	BaseURL           string
}

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		rebuildTaskAliasView()
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*kitdto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.SplitSeq(channel.Group, ",")
		for group := range groups {
			models := channel.GetModels()
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	buildChannelRuntimeSnapshotsLocked(newChannelId2channel)
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	rebuildTaskAliasView()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, filters)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	exactCandidates := group2model2channels[group][model]
	channels, _ := filterCandidateIDs(exactCandidates, model, filters)

	// If no channels found, try to find channels with the normalized model name.
	// 记录「唯一渠道」场景：该分组该模型本只有 1 个候选，但被过滤（冷却/健康分/
	// 任务插件）剔除 → 前端据此提示「唯一渠道过载，建议添加渠道」（G1b）。
	rawCandidateCount := len(exactCandidates)
	if len(channels) == 0 {
		normalizedModel := ratio_setting.RoutingMatchModelName(model)
		normalizedCandidates := group2model2channels[group][normalizedModel]
		rawCandidateCount = len(normalizedCandidates)
		channels, _ = filterCandidateIDs(normalizedCandidates, model, filters)
	}

	if len(channels) == 0 {
		if rawCandidateCount == 1 {
			return nil, errors.New(fmt.Sprintf(
				"only one channel serves this model (group %s, model %s) and it is currently unavailable; consider adding another channel",
				group, model))
		}
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	// 状态变化统一走全量快照重建（只重建索引+解析快照，不碰 channelsIDM 指针，
	// 保留多 key 轮询索引等运行时状态）。相比增量增删索引，全量重建避免了
	// 「冷却到期恢复启用后未重新加回索引导致选不到」的一致性缺口。
	if status == common.ChannelStatusEnabled {
		rebuildGroupIndexesLocked()
	} else {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

// rebuildGroupIndexesLocked 依据 channelsIDM 重建 group2model2channels 索引。
// 调用方必须持有 channelSyncLock 写锁。
func rebuildGroupIndexesLocked() {
	next := make(map[string]map[string][]int)
	groups := make(map[string]bool)
	for _, ch := range channelsIDM {
		for _, g := range ch.GetGroups() {
			groups[g] = true
		}
	}
	// 保持稳定顺序（map 值切片后续不排序也无妨，但选择路径按优先级排序过；
	// 重建后按 id 稳定序，避免随机 map 遍历导致同类渠道选择抖动）。
	groupNames := sortedGroupNames(groups)
	for _, g := range groupNames {
		next[g] = make(map[string][]int)
	}
	for _, ch := range channelsIDM {
		if ch.Status != common.ChannelStatusEnabled {
			continue
		}
		for _, g := range ch.GetGroups() {
			for _, m := range ch.GetModels() {
				next[g][m] = append(next[g][m], ch.Id)
			}
		}
	}
	for g, model2channels := range next {
		for m, ids := range model2channels {
			sort.Slice(ids, func(i, j int) bool {
				a, aok := channelsIDM[ids[i]]
				b, bok := channelsIDM[ids[j]]
				if !aok || !bok {
					return ids[i] < ids[j]
				}
				return a.GetPriority() > b.GetPriority()
			})
			next[g][m] = ids
		}
	}
	group2model2channels = next
}

func sortedGroupNames(groups map[string]bool) []string {
	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Strings(names)
	return names
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}

// buildChannelRuntimeSnapshotsLocked 全量重建渠道运行时快照。调用方必须持有
// channelSyncLock 写锁（InitChannelCache 内调用）。快照只保存“静态”预计算值；
// 解析失败时降级为空值/零值，不阻断缓存重建。
func buildChannelRuntimeSnapshotsLocked(channels map[int]*Channel) {
	next := make(map[int]*ChannelRuntimeSnapshot, len(channels))
	for id, ch := range channels {
		snap := &ChannelRuntimeSnapshot{
			Settings:          ch.GetSetting(),
			OtherSettings:     ch.GetOtherSettings(),
			ModelMapping:      ch.GetModelMapping(),
			StatusCodeMapping: ch.GetStatusCodeMapping(),
			AutoBan:           ch.GetAutoBan(),
			BaseURL:           ch.GetBaseURL(),
		}
		snap.ParamOverride = ch.GetParamOverride()
		snap.HeaderOverride = ch.GetHeaderOverride()
		next[id] = snap
	}
	channelRuntimeSnapshots = next
}

// CacheGetChannelRuntimeSnapshot 返回渠道预计算运行时快照；缓存关闭或未命中时
// 返回 nil（调用方回退到 Channel 方法，保持行为等价）。
func CacheGetChannelRuntimeSnapshot(id int) *ChannelRuntimeSnapshot {
	if !common.MemoryCacheEnabled {
		return nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if channelRuntimeSnapshots == nil {
		return nil
	}
	return channelRuntimeSnapshots[id]
}
