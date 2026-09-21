package setting

import (
	"encoding/base64"
	"slices"
	"sort"
	"strings"

	"github.com/lza6/new-api-Max/common"
)

const (
	TaskPluginMarketplaceSourcesKey  = "TaskPluginMarketplaceSources"
	TaskPluginDisabledFactoryKeysKey = "TaskPluginDisabledFactoryKeys"

	// TaskPluginEd25519PublicKeyKey 是可选的 Ed25519 插件签名公钥（base64，
	// 32 字节）。配置后，上传的自定义插件必须携带有效签名（P2-3）。
	TaskPluginEd25519PublicKeyKey = "TaskPluginEd25519PublicKey"
	// TaskPluginApprovalRequiredKey 表示是否强制「内容寻址审批」后才可执行。
	// 预留开关；开启需配套管理端审批界面（当前版本提供 ExecutionGate 原语，
	// 审批工作流由管理端批次落地）。
	TaskPluginApprovalRequiredKey = "TaskPluginApprovalRequired"

	officialTaskPluginMarketplaceIndexURL = "https://www.newapi.ai/api/v1/plugins/index.json"
	githubTaskPluginMarketplaceIndexURL   = "https://raw.githubusercontent.com/QuantumNous/new-api-plugins/main/index.json"
)

type TaskPluginMarketplaceSource struct {
	Name     string `json:"name"`
	IndexURL string `json:"index_url"`
}

func defaultTaskPluginMarketplaceSources() []TaskPluginMarketplaceSource {
	return []TaskPluginMarketplaceSource{
		{Name: "Official", IndexURL: officialTaskPluginMarketplaceIndexURL},
		{Name: "GitHub", IndexURL: githubTaskPluginMarketplaceIndexURL},
	}
}

func GetTaskPluginMarketplaceSources() []TaskPluginMarketplaceSource {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = common.OptionMap[TaskPluginMarketplaceSourcesKey]
	}
	common.OptionMapRWMutex.RUnlock()

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultTaskPluginMarketplaceSources()
	}
	var sources []TaskPluginMarketplaceSource
	if err := common.UnmarshalJsonStr(raw, &sources); err != nil {
		return defaultTaskPluginMarketplaceSources()
	}
	if sources == nil {
		return []TaskPluginMarketplaceSource{}
	}
	return sources
}

func TaskPluginMarketplaceSources2JsonString() string {
	encoded, err := common.Marshal(defaultTaskPluginMarketplaceSources())
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func ParseTaskPluginDisabledFactoryKeys(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var keys []string
	if err := common.Unmarshal([]byte(raw), &keys); err != nil {
		return []string{}
	}
	if keys == nil {
		return []string{}
	}
	return keys
}

func GetTaskPluginDisabledFactoryKeys() []string {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = common.OptionMap[TaskPluginDisabledFactoryKeysKey]
	}
	common.OptionMapRWMutex.RUnlock()
	return ParseTaskPluginDisabledFactoryKeys(raw)
}

func SetTaskPluginDisabledFactoryKeysOption(keys []string) error {
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	sort.Strings(normalized)
	encoded, err := common.Marshal(normalized)
	if err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[TaskPluginDisabledFactoryKeysKey] = string(encoded)
	common.OptionMapRWMutex.Unlock()
	return nil
}

func IsTaskPluginFactoryDisabled(key string) bool {
	return slices.Contains(GetTaskPluginDisabledFactoryKeys(), key)
}

// GetTaskPluginEd25519PublicKey 返回已配置的插件签名公钥（base64 解码，
// 32 字节）；未配置返回 nil。
func GetTaskPluginEd25519PublicKey() []byte {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = common.OptionMap[TaskPluginEd25519PublicKeyKey]
	}
	common.OptionMapRWMutex.RUnlock()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	return decoded
}

// IsTaskPluginApprovalRequired 返回是否强制内容寻址审批（默认关闭）。
func IsTaskPluginApprovalRequired() bool {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = common.OptionMap[TaskPluginApprovalRequiredKey]
	}
	common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(raw) == "true"
}

// SetTaskPluginEd25519PublicKeyOption 持久化签名公钥（base64）。空值清除。
func SetTaskPluginEd25519PublicKeyOption(publicKey []byte) error {
	raw := ""
	if len(publicKey) > 0 {
		raw = base64.StdEncoding.EncodeToString(publicKey)
	}
	return setTaskPluginOption(TaskPluginEd25519PublicKeyKey, raw)
}

// SetTaskPluginApprovalRequiredOption 持久化审批开关。
func SetTaskPluginApprovalRequiredOption(required bool) error {
	raw := ""
	if required {
		raw = "true"
	}
	return setTaskPluginOption(TaskPluginApprovalRequiredKey, raw)
}

func setTaskPluginOption(key, value string) error {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[key] = value
	return nil
}
