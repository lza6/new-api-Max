package model_setting

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

// QwenSettings defines Qwen model configuration. 注意bool要以enabled结尾才可以生效编辑
type QwenSettings struct {
	SyncImageModels []string `json:"sync_image_models"`
}

// 默认配置
var defaultQwenSettings = QwenSettings{
	SyncImageModels: []string{
		"z-image",
		"qwen-image",
		"wan2.6",
		"wan2.7",
		"qwen-image-edit",
		"qwen-image-edit-max",
		"qwen-image-edit-max-2026-01-16",
		"qwen-image-edit-plus",
		"qwen-image-edit-plus-2025-12-15",
		"qwen-image-edit-plus-2025-10-30",
	},
}

// 全局实例
var qwenSettings = defaultQwenSettings

// qwenSettingsMu 保护 qwenSettings 主副本（SyncImageModels 为 slice）。
var qwenSettingsMu sync.RWMutex

// qwenSettingsSnapshot 已发布的不可变快照。图像模型同步判定走快照，避免与周期
// 热更新（反射就地写 slice）竞争。
var qwenSettingsSnapshot atomic.Pointer[QwenSettings]

// publishQwenSettingsSnapshotLocked 在持 qwenSettingsMu 前提下深拷贝主副本并发布。
func publishQwenSettingsSnapshotLocked() {
	snap := qwenSettings
	snap.SyncImageModels = slices.Clone(qwenSettings.SyncImageModels)
	qwenSettingsSnapshot.Store(&snap)
}

// loadQwenSettings 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadQwenSettings() *QwenSettings {
	if s := qwenSettingsSnapshot.Load(); s != nil {
		return s
	}
	return &qwenSettings
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (q *QwenSettings) BeforeConfigWrite() { qwenSettingsMu.Lock() }
func (q *QwenSettings) AfterConfigWrite() {
	publishQwenSettingsSnapshotLocked()
	qwenSettingsMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (q *QwenSettings) LockConfigRead()   { qwenSettingsMu.RLock() }
func (q *QwenSettings) UnlockConfigRead() { qwenSettingsMu.RUnlock() }

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("qwen", &qwenSettings)
	qwenSettingsMu.Lock()
	publishQwenSettingsSnapshotLocked()
	qwenSettingsMu.Unlock()
}

// GetQwenSettings 返回当前不可变快照。只读，勿直接改写返回对象。
func GetQwenSettings() *QwenSettings {
	return loadQwenSettings()
}

// UpdateQwenSettings 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateQwenSettings(fn func(*QwenSettings)) {
	qwenSettingsMu.Lock()
	defer qwenSettingsMu.Unlock()
	fn(&qwenSettings)
	publishQwenSettingsSnapshotLocked()
}

// IsSyncImageModel
func IsSyncImageModel(model string) bool {
	for _, m := range loadQwenSettings().SyncImageModels {
		if strings.Contains(model, m) {
			return true
		}
	}
	return false
}
