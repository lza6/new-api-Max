package routing_setting

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

// RoutingSetting 复杂度路由的**决策配置**（注册名 "routing"）。
//
// ## 为什么单独一个模块
//
// 打分本身由 `service/complexity_router.go` 负责（纯函数、无状态）。本模块只承载
// 「tier → 候选模型」这份**管理员可编辑的映射**，与 `relay_setting` 的传输参数
// 分离：两者热更新的时机、风险与回滚动作完全不同（改传输参数影响所有请求，
// 改路由映射只影响命中复杂度路由的那部分）。
//
// ## 默认值必须是「什么都不做」
//
// 所有映射默认为空 → shadow 阶段记录「本应路由到哪」但候选为空 → 日志里明确
// 显示 `no_configured_candidates`。这与 G4 的硬约束一致：管理员没有显式配置之前，
// 复杂度路由不产生任何路由决策，也不引入任何误报率。
type RoutingSetting struct {
	// Mode 路由模式："off"（默认，零行为）/ "shadow"（只记录不改变选渠道）/
	// "enforce"（按候选模型前置过滤渠道）。非法值一律按 "off" 处理 —— fail-safe。
	Mode string `json:"mode"`

	// TierModels 档位 → 候选模型（按优先级排列，越靠前越优先）。
	// 键为 "simple" / "medium" / "complex"。空列表 = 该档位不做任何决策。
	TierModels map[string][]string `json:"tier_models"`
}

// RoutingMode 路由模式取值。
const (
	RoutingModeOff     = "off"
	RoutingModeShadow  = "shadow"
	RoutingModeEnforce = "enforce"
)

var routingSetting = RoutingSetting{}

var (
	routingSettingMu       sync.RWMutex
	routingSettingSnapshot atomic.Pointer[RoutingSetting]
)

func publishRoutingSettingSnapshotLocked() {
	snap := routingSetting
	// map 必须深拷贝后发布：否则调用方读到的快照仍与主副本共享同一个 map，
	// 热更新就地写入会构成 map 并发读写（Go 内存模型下是未定义行为，可 fatal）。
	snap.TierModels = maps.Clone(routingSetting.TierModels)
	if snap.TierModels == nil {
		snap.TierModels = map[string][]string{}
	}
	for k, v := range snap.TierModels {
		snap.TierModels[k] = slices.Clone(v)
	}
	routingSettingSnapshot.Store(&snap)
}

// GetRoutingSetting 返回当前配置的**不可变快照**（无锁读）。
func GetRoutingSetting() *RoutingSetting {
	if s := routingSettingSnapshot.Load(); s != nil {
		return s
	}
	routingSettingMu.RLock()
	defer routingSettingMu.RUnlock()
	cp := routingSetting
	cp.TierModels = maps.Clone(routingSetting.TierModels)
	return &cp
}

// UpdateRoutingSetting 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateRoutingSetting(fn func(*RoutingSetting)) {
	routingSettingMu.Lock()
	defer routingSettingMu.Unlock()
	fn(&routingSetting)
	publishRoutingSettingSnapshotLocked()
}

// GetRoutingMode 返回归一化后的路由模式。未知值一律回退 "off"（fail-safe：
// 配置写错时行为等同关闭，而不是意外进入 enforce）。
func GetRoutingMode() string {
	switch GetRoutingSetting().Mode {
	case RoutingModeShadow:
		return RoutingModeShadow
	case RoutingModeEnforce:
		return RoutingModeEnforce
	default:
		return RoutingModeOff
	}
}

// GetTierModels 返回某档位的候选模型副本（无则空切片，调用方无需判 nil）。
func GetTierModels(tier string) []string {
	return slices.Clone(GetRoutingSetting().TierModels[tier])
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook：热更新期间
// 持有写锁，写完发布新快照，使读侧始终读不可变数据。
func (r *RoutingSetting) BeforeConfigWrite() { routingSettingMu.Lock() }
func (r *RoutingSetting) AfterConfigWrite() {
	publishRoutingSettingSnapshotLocked()
	routingSettingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (r *RoutingSetting) LockConfigRead()   { routingSettingMu.RLock() }
func (r *RoutingSetting) UnlockConfigRead() { routingSettingMu.RUnlock() }

func init() {
	config.GlobalConfig.Register("routing", &routingSetting)
	routingSettingMu.Lock()
	publishRoutingSettingSnapshotLocked()
	routingSettingMu.Unlock()
}
