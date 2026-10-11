package service

import (
	"regexp"
	"sync"

	"github.com/lza6/new-api-Max/setting/model_setting"
)

// Chat→Responses upgrade policy is host routing logic (it decides *whether*
// to convert, reading host settings), so it lives here, not in relayconvert.

var chatResponsesRegexCache sync.Map // map[string]*regexp.Regexp

// chatResponsesRegexCacheMaxEntries 是这张缓存的**硬条目上界**（Batch-13 / G10）。
//
// 键 = 配置里的模型匹配模式串。`regexp.Compile` 的结果是确定性的，因此残留条目
// 不会算错，只是永久占内存；这里补上界使「反复改配置」也有确定的内存天花板。
// 达上界后不缓存 —— 只是多编译一次，行为不变。
const chatResponsesRegexCacheMaxEntries = 2048

func matchAnyModelPattern(patterns []string, model string) bool {
	if len(patterns) == 0 || model == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := chatResponsesRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				// Treat invalid patterns as non-matching to avoid breaking runtime traffic.
				continue
			}
			if chatResponsesRegexCacheSize() < chatResponsesRegexCacheMaxEntries {
				chatResponsesRegexCache.Store(pattern, compiled)
			}
			re = compiled
		}
		if re.(*regexp.Regexp).MatchString(model) {
			return true
		}
	}
	return false
}

// chatResponsesRegexCacheSize 数一遍缓存条目数（仅在未命中路径调用）。
func chatResponsesRegexCacheSize() int {
	n := 0
	chatResponsesRegexCache.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	return matchAnyModelPattern(policy.ModelPatterns, model)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		channelID,
		channelType,
		model,
	)
}
