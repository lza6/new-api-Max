package model_setting

import (
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

const defaultGeminiSafetySetting = "OFF"

var validGeminiSafetySettings = map[string]struct{}{
	"OFF":                              {},
	"BLOCK_NONE":                       {},
	"BLOCK_ONLY_HIGH":                  {},
	"BLOCK_MEDIUM_AND_ABOVE":           {},
	"BLOCK_LOW_AND_ABOVE":              {},
	"HARM_BLOCK_THRESHOLD_UNSPECIFIED": {},
}

// GeminiSettings defines Gemini model configuration. 注意bool要以enabled结尾才可以生效编辑
type GeminiSettings struct {
	SafetySettings                        map[string]string `json:"safety_settings"`
	VersionSettings                       map[string]string `json:"version_settings"`
	SupportedImagineModels                []string          `json:"supported_imagine_models"`
	ThinkingAdapterEnabled                bool              `json:"thinking_adapter_enabled"`
	ThinkingAdapterBudgetTokensPercentage float64           `json:"thinking_adapter_budget_tokens_percentage"`
	FunctionCallThoughtSignatureEnabled   bool              `json:"function_call_thought_signature_enabled"`
	RemoveFunctionResponseIdEnabled       bool              `json:"remove_function_response_id_enabled"`
}

// 默认配置
var defaultGeminiSettings = GeminiSettings{
	SafetySettings: map[string]string{
		"default": defaultGeminiSafetySetting,
	},
	VersionSettings: map[string]string{
		"default":        "v1beta",
		"gemini-1.0-pro": "v1",
	},
	SupportedImagineModels: []string{
		"gemini-2.0-flash-exp-image-generation",
		"gemini-2.0-flash-exp",
		"gemini-3-pro-image-preview",
		"gemini-3-pro-image",
		"gemini-2.5-flash-image",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
	},
	ThinkingAdapterEnabled:                false,
	ThinkingAdapterBudgetTokensPercentage: 0.6,
	FunctionCallThoughtSignatureEnabled:   true,
	RemoveFunctionResponseIdEnabled:       true,
}

// 全局实例
var geminiSettings = defaultGeminiSettings

// geminiSettingsMu 保护 geminiSettings 主副本（两个 map + 一个 slice）。
var geminiSettingsMu sync.RWMutex

// geminiSettingsSnapshot 已发布的不可变快照。中继热路径（安全阈值/版本/图像模型
// 判定）只读快照，避免与周期热更新（反射就地写 map/slice）竞争。
var geminiSettingsSnapshot atomic.Pointer[GeminiSettings]

// publishGeminiSettingsSnapshotLocked 在持 geminiSettingsMu 前提下深拷贝主副本并发布。
func publishGeminiSettingsSnapshotLocked() {
	snap := geminiSettings
	snap.SafetySettings = maps.Clone(geminiSettings.SafetySettings)
	snap.VersionSettings = maps.Clone(geminiSettings.VersionSettings)
	snap.SupportedImagineModels = slices.Clone(geminiSettings.SupportedImagineModels)
	geminiSettingsSnapshot.Store(&snap)
}

// loadGeminiSettings 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadGeminiSettings() *GeminiSettings {
	if s := geminiSettingsSnapshot.Load(); s != nil {
		return s
	}
	return &geminiSettings
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (g *GeminiSettings) BeforeConfigWrite() { geminiSettingsMu.Lock() }
func (g *GeminiSettings) AfterConfigWrite() {
	publishGeminiSettingsSnapshotLocked()
	geminiSettingsMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (g *GeminiSettings) LockConfigRead()   { geminiSettingsMu.RLock() }
func (g *GeminiSettings) UnlockConfigRead() { geminiSettingsMu.RUnlock() }

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("gemini", &geminiSettings)
	geminiSettingsMu.Lock()
	publishGeminiSettingsSnapshotLocked()
	geminiSettingsMu.Unlock()
}

// GetGeminiSettings 返回当前不可变快照。只读，勿直接改写返回对象。
func GetGeminiSettings() *GeminiSettings {
	return loadGeminiSettings()
}

// UpdateGeminiSettings 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateGeminiSettings(fn func(*GeminiSettings)) {
	geminiSettingsMu.Lock()
	defer geminiSettingsMu.Unlock()
	fn(&geminiSettings)
	publishGeminiSettingsSnapshotLocked()
}

// GetGeminiSafetySetting 获取安全设置
func GetGeminiSafetySetting(key string) string {
	settings := loadGeminiSettings().SafetySettings
	if value := settings[key]; value != "" {
		return value
	}
	if value := settings["default"]; value != "" {
		return value
	}
	return defaultGeminiSafetySetting
}

// ValidateGeminiSafetySettings validates the JSON persisted by the option API.
// Empty values remain valid because read-time fallback returns the default.
func ValidateGeminiSafetySettings(value string) error {
	var settings map[string]string
	if err := common.UnmarshalJsonStr(value, &settings); err != nil {
		return fmt.Errorf("Gemini safety settings must be a JSON string map: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("Gemini safety settings must be a JSON string map")
	}
	for category, threshold := range settings {
		if threshold == "" {
			continue
		}
		if _, ok := validGeminiSafetySettings[threshold]; !ok {
			return fmt.Errorf("invalid Gemini safety threshold %q for %q", threshold, category)
		}
	}
	return nil
}

// GetGeminiVersionSetting 获取版本设置
func GetGeminiVersionSetting(key string) string {
	settings := loadGeminiSettings().VersionSettings
	if value, ok := settings[key]; ok {
		return value
	}
	return settings["default"]
}

func IsGeminiModelSupportImagine(model string) bool {
	return slices.Contains(loadGeminiSettings().SupportedImagineModels, model)
}
