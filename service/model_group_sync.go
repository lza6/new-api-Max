package service

import (
	"strings"

	"github.com/lza6/new-api-Max/model"
)

// SyncModelGroupToChannels T2 模型分组归类：把某模型的 Groups 设置同步到
// 所有「Models 含该模型」的渠道：取渠道现有 group 与模型 groups 的并集
// 写回渠道 Group，并重建 abilities，使该模型在指定分组下真实可用。
// 只做并集（加组），不做减法，避免误伤同一渠道上的其他模型分组。
func SyncModelGroupToChannels(modelName string, modelGroups []string) (updated int, err error) {
	cleanGroups := normalizeGroupList(modelGroups)
	if len(cleanGroups) == 0 {
		cleanGroups = []string{"default"}
	}

	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return 0, err
	}

	updatedChannels := 0
	for _, channel := range channels {
		if channel.Status != 1 { // 只同步启用渠道
			continue
		}
		if !channelContainsModel(channel, modelName) {
			continue
		}
		channelGroups := channel.GetGroups()
		merged := mergeGroups(channelGroups, cleanGroups)
		if groupsEqual(channelGroups, merged) {
			continue
		}
		channel.Group = strings.Join(merged, ",")
		if err := channel.Update(); err != nil {
			return updatedChannels, err
		}
		updatedChannels++
	}
	return updatedChannels, nil
}

func channelContainsModel(channel *model.Channel, modelName string) bool {
	for _, name := range channel.GetModels() {
		if strings.TrimSpace(name) == modelName {
			return true
		}
	}
	return false
}

// normalizeGroupList 去空、去重。
func normalizeGroupList(groups []string) []string {
	seen := make(map[string]struct{}, len(groups))
	result := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		result = append(result, g)
	}
	return result
}

func mergeGroups(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))
	for _, g := range a {
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		result = append(result, g)
	}
	for _, g := range b {
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		result = append(result, g)
	}
	return result
}

func groupsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
