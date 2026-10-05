package model_setting

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled bool     `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist    []string `json:"thinking_model_blacklist"`
	// EffortTailModelIDs lists real model IDs that sit inside the GPT/o-series
	// family whitelist but whose names already end in an effort word.
	EffortTailModelIDs               []string                         `json:"effort_tail_model_ids"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	EffortTailModelIDs: []string{
		"gpt-5.1-codex-max",
		"qwen-image-edit-max",
		"qwen-max",
		"stable-diffusion-3-medium",
		"yi-medium",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
}

// 全局实例
var globalSettings = defaultOpenaiSettings

// globalSettingsMu 保护 globalSettings 主副本（两个 []string + 嵌套策略的 slice）。
var globalSettingsMu sync.RWMutex

// globalSettingsSnapshot 已发布的不可变快照。中继热路径（透传判定 / 思考后缀 /
// effort 后缀）只读快照，避免与周期热更新（反射就地写 slice）竞争。
var globalSettingsSnapshot atomic.Pointer[GlobalSettings]

// cloneChatCompletionsToResponsesPolicy 深拷贝策略内的两个 slice。
func cloneChatCompletionsToResponsesPolicy(policy ChatCompletionsToResponsesPolicy) ChatCompletionsToResponsesPolicy {
	policy.ChannelIDs = slices.Clone(policy.ChannelIDs)
	policy.ChannelTypes = slices.Clone(policy.ChannelTypes)
	policy.ModelPatterns = slices.Clone(policy.ModelPatterns)
	return policy
}

// publishGlobalSettingsSnapshotLocked 在持 globalSettingsMu 前提下深拷贝主副本并发布。
func publishGlobalSettingsSnapshotLocked() {
	snap := globalSettings
	snap.ThinkingModelBlacklist = slices.Clone(globalSettings.ThinkingModelBlacklist)
	snap.EffortTailModelIDs = slices.Clone(globalSettings.EffortTailModelIDs)
	snap.ChatCompletionsToResponsesPolicy = cloneChatCompletionsToResponsesPolicy(globalSettings.ChatCompletionsToResponsesPolicy)
	globalSettingsSnapshot.Store(&snap)
}

// loadGlobalSettings 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadGlobalSettings() *GlobalSettings {
	if s := globalSettingsSnapshot.Load(); s != nil {
		return s
	}
	return &globalSettings
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (g *GlobalSettings) BeforeConfigWrite() { globalSettingsMu.Lock() }
func (g *GlobalSettings) AfterConfigWrite() {
	publishGlobalSettingsSnapshotLocked()
	globalSettingsMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (g *GlobalSettings) LockConfigRead()   { globalSettingsMu.RLock() }
func (g *GlobalSettings) UnlockConfigRead() { globalSettingsMu.RUnlock() }

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
	globalSettingsMu.Lock()
	publishGlobalSettingsSnapshotLocked()
	globalSettingsMu.Unlock()
}

// GetGlobalSettings 返回当前不可变快照。只读，勿直接改写返回对象。
func GetGlobalSettings() *GlobalSettings {
	return loadGlobalSettings()
}

// UpdateGlobalSettings 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateGlobalSettings(fn func(*GlobalSettings)) {
	globalSettingsMu.Lock()
	defer globalSettingsMu.Unlock()
	fn(&globalSettings)
	publishGlobalSettingsSnapshotLocked()
}

const thinkingBlacklistRegexPrefix = "re:"

type thinkingBlacklistCompiled struct {
	source  string
	exact   []string
	regexes []*regexp.Regexp
}

var (
	thinkingBlacklistMu    sync.RWMutex
	thinkingBlacklistCache thinkingBlacklistCompiled
)

func thinkingBlacklistSourceKey(entries []string) string {
	return strings.Join(entries, "\x00")
}

func compiledThinkingBlacklist() ([]string, []*regexp.Regexp) {
	entries := loadGlobalSettings().ThinkingModelBlacklist
	key := thinkingBlacklistSourceKey(entries)

	thinkingBlacklistMu.RLock()
	if thinkingBlacklistCache.source == key {
		exact, regexes := thinkingBlacklistCache.exact, thinkingBlacklistCache.regexes
		thinkingBlacklistMu.RUnlock()
		return exact, regexes
	}
	thinkingBlacklistMu.RUnlock()

	thinkingBlacklistMu.Lock()
	defer thinkingBlacklistMu.Unlock()
	if thinkingBlacklistCache.source == key {
		return thinkingBlacklistCache.exact, thinkingBlacklistCache.regexes
	}

	exact := make([]string, 0, len(entries))
	var regexes []*regexp.Regexp
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if after, ok := strings.CutPrefix(entry, thinkingBlacklistRegexPrefix); ok {
			pattern := after
			if pattern == "" {
				common.SysError(fmt.Sprintf("invalid thinking_model_blacklist regex %q: pattern is empty", entry))
				continue
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				common.SysError(fmt.Sprintf("invalid thinking_model_blacklist regex %q: %v", entry, err))
				continue
			}
			regexes = append(regexes, re)
			continue
		}
		exact = append(exact, entry)
	}
	thinkingBlacklistCache = thinkingBlacklistCompiled{source: key, exact: exact, regexes: regexes}
	return exact, regexes
}

// ShouldPreserveThinkingSuffix reports whether the full model name is exempt
// from host thinking-suffix and @-modifier parsing. Exact blacklist entries
// match the complete name; entries prefixed with re: are Go regular expressions
// matched with MatchString against the same full name.
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	exact, regexes := compiledThinkingBlacklist()
	if slices.Contains(exact, target) {
		return true
	}
	for _, re := range regexes {
		if re.MatchString(target) {
			return true
		}
	}
	return false
}

// ShouldPreserveEffortTail reports whether modelName is a real model ID whose
// name already ends in an effort word. Entries match the complete name and the
// de-namespaced bare name.
func ShouldPreserveEffortTail(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}
	bare := target
	if slash := strings.LastIndex(target, "/"); slash >= 0 {
		bare = target[slash+1:]
	}

	for _, entry := range loadGlobalSettings().EffortTailModelIDs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == target || entry == bare {
			return true
		}
	}
	return false
}
