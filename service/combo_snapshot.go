package service

// P2-1 无锁配置快照：渠道组合（ChannelCombo）只读索引。
//
// ResolveComboForModel 原本每次请求直查数据库（GetEnabledComboByName），
// 在组合路由热点下是无谓的读放大。这里把「启用组合名 -> *ChannelCombo」
// 构建为不可变快照，用 atomic.Pointer 原子发布：
//
//   - 读路径（Snapshot/Resolve）零锁：Load 一次取指针，只读 map，永不修改。
//   - 写路径（RefreshComboSnapshot）全量重建新 map 后 Store 替换；旧快照
//     继续被并发读者安全使用（GC 回收）。
//   - 失效：组合 Create/Update/Delete 后调用 RefreshComboSnapshot。
//
// 无需引入 RWMutex：读多写少、快照不可变，这正是 atomic.Pointer 的适用场景。

import (
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/model"
)

// comboSnapshot 是不可变只读索引：name -> 启用组合。
type comboSnapshot struct {
	byName map[string]*model.ChannelCombo
}

var (
	comboSnapshotPtr atomic.Pointer[comboSnapshot]
	comboSnapshotMu  sync.Mutex // 仅保护重建本身，避免并发重复全量查询
)

// comboSnapshotIndex 从全量启用组合构建索引快照（nil-safe）。本函数不发布，
// 由 RefreshComboSnapshot 负责原子替换。
func buildComboSnapshot(combos []*model.ChannelCombo) *comboSnapshot {
	if len(combos) == 0 {
		return &comboSnapshot{byName: map[string]*model.ChannelCombo{}}
	}
	index := make(map[string]*model.ChannelCombo, len(combos))
	for _, c := range combos {
		if c == nil || c.Name == "" {
			continue
		}
		index[c.Name] = c
	}
	return &comboSnapshot{byName: index}
}

// LoadComboSnapshot 返回当前快照（无锁；不存在时返回空快照，不触发查询）。
func LoadComboSnapshot() *comboSnapshot {
	if s := comboSnapshotPtr.Load(); s != nil {
		return s
	}
	return &comboSnapshot{byName: map[string]*model.ChannelCombo{}}
}

// RefreshComboSnapshot 全量重建并原子发布组合快照。失败时保留旧快照
// （读端不受影响）。串行化重建避免并发下重复全表查询。
func RefreshComboSnapshot() error {
	comboSnapshotMu.Lock()
	defer comboSnapshotMu.Unlock()
	combos, err := model.ListEnabledCombos()
	if err != nil {
		return err
	}
	comboSnapshotPtr.Store(buildComboSnapshot(combos))
	return nil
}

// ResolveComboSnapshot 无锁解析启用组合（nil = 无/非组合）。这是
// ResolveComboForModel 的快照路径，避免每请求查询数据库。
func ResolveComboSnapshot(modelName string) *model.ChannelCombo {
	if modelName == "" {
		return nil
	}
	return LoadComboSnapshot().byName[modelName]
}
