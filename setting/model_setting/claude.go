package model_setting

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

//var claudeHeadersSettings = map[string][]string{}
//
//var ClaudeThinkingAdapterEnabled = true
//var ClaudeThinkingAdapterMaxTokens = 8192
//var ClaudeThinkingAdapterBudgetTokensPercentage = 0.8

// ClaudeSettings 定义Claude模型的配置
type ClaudeSettings struct {
	HeadersSettings                       map[string]map[string][]string `json:"model_headers_settings"`
	DefaultMaxTokens                      map[string]int                 `json:"default_max_tokens"`
	ThinkingAdapterEnabled                bool                           `json:"thinking_adapter_enabled"`
	ThinkingAdapterBudgetTokensPercentage float64                        `json:"thinking_adapter_budget_tokens_percentage"`
}

// 默认配置
var defaultClaudeSettings = ClaudeSettings{
	HeadersSettings:        map[string]map[string][]string{},
	ThinkingAdapterEnabled: true,
	DefaultMaxTokens: map[string]int{
		"default": 8192,
	},
	ThinkingAdapterBudgetTokensPercentage: 0.8,
}

// 全局实例
var claudeSettings = defaultClaudeSettings

// claudeSettingsMu 保护 claudeSettings 主副本（HeadersSettings 为嵌套 map、
// DefaultMaxTokens 为 map）。写入持写锁并在写完后发布新快照。
var claudeSettingsMu sync.RWMutex

// claudeSettingsSnapshot 已发布的不可变快照。中继热路径（WriteHeaders /
// GetDefaultMaxTokens）只读快照，避免与 60s 周期热更新（反射就地写 map）竞争。
var claudeSettingsSnapshot atomic.Pointer[ClaudeSettings]

// cloneClaudeHeadersSettings 深拷贝嵌套 map（model -> header -> values）。
// 内层 slice 也必须克隆，否则快照与主副本共享底层数组。
func cloneClaudeHeadersSettings(src map[string]map[string][]string) map[string]map[string][]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]map[string][]string, len(src))
	for model, headers := range src {
		if headers == nil {
			dst[model] = nil
			continue
		}
		clone := make(map[string][]string, len(headers))
		for key, values := range headers {
			clone[key] = slices.Clone(values)
		}
		dst[model] = clone
	}
	return dst
}

// publishClaudeSettingsSnapshotLocked 在持 claudeSettingsMu 前提下深拷贝主副本并发布。
func publishClaudeSettingsSnapshotLocked() {
	snap := claudeSettings
	snap.HeadersSettings = cloneClaudeHeadersSettings(claudeSettings.HeadersSettings)
	snap.DefaultMaxTokens = maps.Clone(claudeSettings.DefaultMaxTokens)
	claudeSettingsSnapshot.Store(&snap)
}

// loadClaudeSettings 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadClaudeSettings() *ClaudeSettings {
	if s := claudeSettingsSnapshot.Load(); s != nil {
		return s
	}
	return &claudeSettings
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (c *ClaudeSettings) BeforeConfigWrite() { claudeSettingsMu.Lock() }
func (c *ClaudeSettings) AfterConfigWrite() {
	publishClaudeSettingsSnapshotLocked()
	claudeSettingsMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (c *ClaudeSettings) LockConfigRead()   { claudeSettingsMu.RLock() }
func (c *ClaudeSettings) UnlockConfigRead() { claudeSettingsMu.RUnlock() }

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("claude", &claudeSettings)
	claudeSettingsMu.Lock()
	publishClaudeSettingsSnapshotLocked()
	claudeSettingsMu.Unlock()
}

// GetClaudeSettings 返回当前不可变快照。只读，勿直接改写返回对象。
func GetClaudeSettings() *ClaudeSettings {
	s := loadClaudeSettings()
	// check default max tokens must have default key
	if _, ok := s.DefaultMaxTokens["default"]; !ok {
		UpdateClaudeSettings(func(settings *ClaudeSettings) {
			if settings.DefaultMaxTokens == nil {
				settings.DefaultMaxTokens = make(map[string]int, 1)
			}
			settings.DefaultMaxTokens["default"] = 8192
		})
		return loadClaudeSettings()
	}
	return s
}

// UpdateClaudeSettings 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateClaudeSettings(fn func(*ClaudeSettings)) {
	claudeSettingsMu.Lock()
	defer claudeSettingsMu.Unlock()
	fn(&claudeSettings)
	publishClaudeSettingsSnapshotLocked()
}

func (c *ClaudeSettings) WriteHeaders(originModel string, httpHeader *http.Header) {
	if headers, ok := c.HeadersSettings[originModel]; ok {
		for headerKey, headerValues := range headers {
			mergedValues := normalizeHeaderListValues(
				append(append([]string(nil), httpHeader.Values(headerKey)...), headerValues...),
			)
			if len(mergedValues) == 0 {
				continue
			}
			httpHeader.Set(headerKey, strings.Join(mergedValues, ","))
		}
	}
}

func normalizeHeaderListValues(values []string) []string {
	normalizedValues := make([]string, 0, len(values))
	seenValues := make(map[string]struct{}, len(values))
	for _, value := range values {
		for item := range strings.SplitSeq(value, ",") {
			normalizedItem := strings.TrimSpace(item)
			if normalizedItem == "" {
				continue
			}
			if _, exists := seenValues[normalizedItem]; exists {
				continue
			}
			seenValues[normalizedItem] = struct{}{}
			normalizedValues = append(normalizedValues, normalizedItem)
		}
	}
	return normalizedValues
}

func (c *ClaudeSettings) GetDefaultMaxTokens(model string) int {
	if maxTokens, ok := c.DefaultMaxTokens[model]; ok {
		return maxTokens
	}
	return c.DefaultMaxTokens["default"]
}

// ValidateClaudeDefaultMaxTokens validates the JSON persisted by the option
// API. Zero stays allowed — the current Messages API accepts max_tokens: 0 as
// cache pre-warming — but negative values are rejected because they would
// wrap into huge unsigned values during request conversion.
func ValidateClaudeDefaultMaxTokens(value string) error {
	var settings map[string]int
	if err := common.UnmarshalJsonStr(value, &settings); err != nil {
		return fmt.Errorf("Claude default max tokens must be a JSON map of model to integer: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("Claude default max tokens must be a JSON map of model to integer")
	}
	for model, maxTokens := range settings {
		if maxTokens < 0 {
			return fmt.Errorf("negative Claude default max_tokens %d for %q", maxTokens, model)
		}
	}
	return nil
}
