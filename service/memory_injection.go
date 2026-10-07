package service

import (
	"strings"

	"github.com/lza6/new-api-Max/common"
)

// T8 记忆注入（用户级 system prompt 注入）——T8 「画像/记忆中间件」的**注入半边**。
//
// 现状盘点：本项目已有
//   - 只读画像：`service/user_profile/profile.go`（规则版画像 + 衰减 + 缓存）
//   - 轻量记忆：`service/user_memory.go`（RecordUserLastModel）
//   - 渠道级 system prompt 注入：`info.ChannelSetting.SystemPrompt`（按格式各写一遍）
//
// 本文件补的是**用户级**：让用户把「自己的记忆/偏好片段」注入到发给上游的 system
// prompt 里（个性化）。默认**关**（MEMORY_INJECTION_ENABLED），且仅对**显式开启**
// 该开关的部署生效——避免影响所有用户的请求语义。
//
// 设计原则：纯函数构建注入片段，调用方决定插入位置；不在此处改 DTO。

// memoryInjectionEnabled 全局开关：MEMORY_INJECTION_ENABLED=on|true。默认关。
var memoryInjectionEnabled = common.GetEnvOrDefaultBool("MEMORY_INJECTION_ENABLED", false)

// memoryInjectionMaxBytes 注入片段上限（字节），防用户塞超长内容撑爆请求。
const memoryInjectionMaxBytes = 4096

// SetMemoryInjectionEnabled 测试用覆盖；nil 恢复 env 默认。
func SetMemoryInjectionEnabled(v *bool) {
	if v == nil {
		memoryInjectionEnabled = common.GetEnvOrDefaultBool("MEMORY_INJECTION_ENABLED", false)
		return
	}
	memoryInjectionEnabled = *v
}

// MemoryInjectionEnabled 报告记忆注入开关状态。
func MemoryInjectionEnabled() bool { return memoryInjectionEnabled }

// BuildMemoryInjection 构建要注入的 system 片段（纯函数）。
//   - userMemory：用户配置的记忆片段（来自 UserSetting；空则不注入）。
//   - 超过上限时截断到 memoryInjectionMaxBytes（rune 边界安全）。
//
// 返回空串表示不注入。调用方据返回非空决定是否插入 system。
func BuildMemoryInjection(userMemory string) string {
	if !memoryInjectionEnabled {
		return ""
	}
	userMemory = strings.TrimSpace(userMemory)
	if userMemory == "" {
		return ""
	}
	if len(userMemory) > memoryInjectionMaxBytes {
		b := []byte(userMemory)[:memoryInjectionMaxBytes]
		for len(b) > 0 && b[len(b)-1]&0xC0 == 0x80 {
			b = b[:len(b)-1]
		}
		userMemory = string(b)
	}
	return userMemory
}

// MergeMemoryIntoSystem 把记忆片段合并进一个「字符串 system prompt」（纯函数）。
// 语义与渠道级注入一致：记忆拼在已有 system **之前**（让用户记忆作为稳定前缀，
// 利于上游前缀缓存命中）。空记忆 → 原样返回。
func MergeMemoryIntoSystem(existingSystem, userMemory string) string {
	inj := BuildMemoryInjection(userMemory)
	if inj == "" {
		return existingSystem
	}
	existingSystem = strings.TrimSpace(existingSystem)
	if existingSystem == "" {
		return inj
	}
	return inj + "\n" + existingSystem
}
