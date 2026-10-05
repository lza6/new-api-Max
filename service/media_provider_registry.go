package service

import (
	"sort"
	"sync"

	"github.com/lza6/new-api-Max/constant"
)

// §4.6.1 媒体 Provider 能力注册表（统一媒体适配层的最小落地）。
//
// 现状：本仓已有成熟的适配器范型（`relay/channel` 的 `Adaptor` / `TaskAdaptor` 接口 +
// `relay.GetAdaptor` / `relay.GetTaskAdaptor` 工厂），41 个渠道目录与 10 个 task 插件
// 各自实现。缺口不是「没有适配器」，而是缺少**统一的媒体能力目录**——无法在一个地方
// 查询「谁支持文生视频/图生视频/TTS/图像生成」。
//
// 本注册表是**只读能力目录**（metadata），不改变任何分发逻辑：
//   - 各渠道/插件可在 init 时 `RegisterMediaProvider(...)` 声明能力；
//   - 查询 `MediaProvidersFor(capability)` 得到 provider 列表；
//   - `SupportsMediaCapability(channelType, capability)` 供上层做能力门控/文档生成。
//
// 这为「新增一个媒体上游只需实现接口 + 注册」提供统一入口，且**零分发改动、零回归风险**。
// 后续可将分发也收敛到注册表（迁移期保持两套并存）。
type MediaCapability string

const (
	MediaCapabilityTextToVideo  MediaCapability = "text_to_video"
	MediaCapabilityImageToVideo MediaCapability = "image_to_video"
	MediaCapabilityImageGen     MediaCapability = "image_generation"
	MediaCapabilityTTS          MediaCapability = "tts"
	MediaCapabilityASR          MediaCapability = "asr"
)

// MediaProvider 描述一个媒体能力提供者（渠道类型或 task 平台）。
type MediaProvider struct {
	// Name 稳定标识（如 "kling" / "jimeng"），用于日志与文档。
	Name string
	// ChannelType 渠道类型常量（>0 时用于按渠道能力门控）。
	ChannelType int
	// Platform task 平台（非空时用于任务分发能力查询）。
	Platform constant.TaskPlatform
	// Capabilities 该提供者支持的媒体能力集合。
	Capabilities []MediaCapability
}

var (
	mediaProviderMu       sync.RWMutex
	mediaProviderRegistry = map[string]MediaProvider{}
)

// RegisterMediaProvider 注册一个媒体能力提供者（幂等：同名覆盖）。供各渠道/插件 init 调用。
func RegisterMediaProvider(p MediaProvider) {
	if p.Name == "" {
		return
	}
	mediaProviderMu.Lock()
	mediaProviderRegistry[p.Name] = p
	mediaProviderMu.Unlock()
}

// MediaProvidersFor 返回支持指定媒体能力的全部提供者（按 Name 排序，稳定输出）。
func MediaProvidersFor(capability MediaCapability) []MediaProvider {
	mediaProviderMu.RLock()
	defer mediaProviderMu.RUnlock()
	out := make([]MediaProvider, 0, len(mediaProviderRegistry))
	for _, p := range mediaProviderRegistry {
		for _, c := range p.Capabilities {
			if c == capability {
				out = append(out, p)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SupportsMediaCapability 报告某渠道类型是否支持指定媒体能力。
func SupportsMediaCapability(channelType int, capability MediaCapability) bool {
	mediaProviderMu.RLock()
	defer mediaProviderMu.RUnlock()
	for _, p := range mediaProviderRegistry {
		if p.ChannelType != channelType {
			continue
		}
		for _, c := range p.Capabilities {
			if c == capability {
				return true
			}
		}
	}
	return false
}

// AllMediaProviders 返回注册表快照（供文档生成/管理端展示）。
func AllMediaProviders() []MediaProvider {
	mediaProviderMu.RLock()
	defer mediaProviderMu.RUnlock()
	out := make([]MediaProvider, 0, len(mediaProviderRegistry))
	for _, p := range mediaProviderRegistry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// resetMediaProviderRegistryForTest 清空注册表（测试用）。
func resetMediaProviderRegistryForTest() {
	mediaProviderMu.Lock()
	mediaProviderRegistry = map[string]MediaProvider{}
	mediaProviderMu.Unlock()
}

// init 用既有渠道常量登记已知的媒体提供者（能力目录的初始数据，随实际适配器演进）。
func init() {
	// 任务类平台（视频/图像生成）——与 relay.GetTaskAdaptor 的平台映射一致。
	RegisterMediaProvider(MediaProvider{Name: "kling", ChannelType: constant.ChannelTypeKling, Capabilities: []MediaCapability{MediaCapabilityTextToVideo, MediaCapabilityImageToVideo}})
	RegisterMediaProvider(MediaProvider{Name: "jimeng", ChannelType: constant.ChannelTypeJimeng, Capabilities: []MediaCapability{MediaCapabilityTextToVideo, MediaCapabilityImageToVideo}})
	RegisterMediaProvider(MediaProvider{Name: "vidu", ChannelType: constant.ChannelTypeVidu, Capabilities: []MediaCapability{MediaCapabilityTextToVideo, MediaCapabilityImageToVideo}})
	RegisterMediaProvider(MediaProvider{Name: "doubao", ChannelType: constant.ChannelTypeDoubaoVideo, Capabilities: []MediaCapability{MediaCapabilityTextToVideo, MediaCapabilityImageToVideo}})
	RegisterMediaProvider(MediaProvider{Name: "sora", ChannelType: constant.ChannelTypeSora, Capabilities: []MediaCapability{MediaCapabilityTextToVideo}})
	RegisterMediaProvider(MediaProvider{Name: "volcengine", ChannelType: constant.ChannelTypeVolcEngine, Capabilities: []MediaCapability{MediaCapabilityTextToVideo, MediaCapabilityImageToVideo}})
	RegisterMediaProvider(MediaProvider{Name: "suno", ChannelType: constant.ChannelTypeSunoAPI, Capabilities: []MediaCapability{MediaCapabilityTTS}})
	RegisterMediaProvider(MediaProvider{Name: "midjourney", ChannelType: constant.ChannelTypeMidjourney, Capabilities: []MediaCapability{MediaCapabilityImageGen}})
}
