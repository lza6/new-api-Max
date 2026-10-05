package task_pricing_setting

import (
	"maps"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

type TaskPricingSetting struct {
	SoraSizeRatio      map[string]float64 `json:"sora_size_ratio"`
	VertexResolution4K map[string]float64 `json:"vertex_resolution_4k_ratio"`
}

var taskPricingSetting = TaskPricingSetting{
	SoraSizeRatio: map[string]float64{
		"1792x1024": 1.666667,
		"1024x1792": 1.666667,
	},
	VertexResolution4K: map[string]float64{
		"veo-3.1-fast-generate": 2.333333,
		"veo-3.1-generate":      1.5,
		"veo-3.1":               1.5,
	},
}

// taskPricingSettingMu 保护 taskPricingSetting 主副本（两个 map）。
var taskPricingSettingMu sync.RWMutex

// taskPricingSettingSnapshot 已发布的不可变快照。任务结算计价热路径只读快照，
// 避免与周期热更新（反射就地写 map）竞争。
var taskPricingSettingSnapshot atomic.Pointer[TaskPricingSetting]

// publishTaskPricingSettingSnapshotLocked 在持 taskPricingSettingMu 前提下深拷贝
// 主副本并发布。
func publishTaskPricingSettingSnapshotLocked() {
	snap := taskPricingSetting
	snap.SoraSizeRatio = maps.Clone(taskPricingSetting.SoraSizeRatio)
	snap.VertexResolution4K = maps.Clone(taskPricingSetting.VertexResolution4K)
	taskPricingSettingSnapshot.Store(&snap)
}

// loadTaskPricingSetting 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadTaskPricingSetting() *TaskPricingSetting {
	if s := taskPricingSettingSnapshot.Load(); s != nil {
		return s
	}
	return &taskPricingSetting
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (t *TaskPricingSetting) BeforeConfigWrite() { taskPricingSettingMu.Lock() }
func (t *TaskPricingSetting) AfterConfigWrite() {
	publishTaskPricingSettingSnapshotLocked()
	taskPricingSettingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (t *TaskPricingSetting) LockConfigRead()   { taskPricingSettingMu.RLock() }
func (t *TaskPricingSetting) UnlockConfigRead() { taskPricingSettingMu.RUnlock() }

// UpdateTaskPricingSetting 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateTaskPricingSetting(fn func(*TaskPricingSetting)) {
	taskPricingSettingMu.Lock()
	defer taskPricingSettingMu.Unlock()
	fn(&taskPricingSetting)
	publishTaskPricingSettingSnapshotLocked()
}

func init() {
	config.GlobalConfig.Register("task_pricing_setting", &taskPricingSetting)
	taskPricingSettingMu.Lock()
	publishTaskPricingSettingSnapshotLocked()
	taskPricingSettingMu.Unlock()
}

func SoraSizeRatio(size string) float64 {
	if ratio, ok := loadTaskPricingSetting().SoraSizeRatio[size]; ok && ratio > 0 {
		return ratio
	}
	return 1
}

func VertexResolutionRatio(model, resolution string) float64 {
	if !strings.EqualFold(resolution, "4k") {
		return 1
	}
	matchedPattern := ""
	matchedRatio := 1.0
	for pattern, ratio := range loadTaskPricingSetting().VertexResolution4K {
		if !strings.Contains(model, pattern) || ratio <= 0 {
			continue
		}
		if len(pattern) > len(matchedPattern) || (len(pattern) == len(matchedPattern) && pattern < matchedPattern) {
			matchedPattern = pattern
			matchedRatio = ratio
		}
	}
	return matchedRatio
}

// GetCopy 返回当前配置的深拷贝（只读快照）。
func GetCopy() TaskPricingSetting {
	return *loadTaskPricingSetting()
}
