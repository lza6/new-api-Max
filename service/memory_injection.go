package service

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
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

// MemoryInjectionEnabled 全局开关：MEMORY_INJECTION_ENABLED=on|true。默认关。
// 值来源：管理端「实验功能」页持久化配置 > env MEMORY_INJECTION_ENABLED（默认 false）。
var memoryInjectionEnvDefault = common.GetEnvOrDefaultBool(common.FlagMemoryInjectionEnabled, false)

// memoryInjectionMaxBytes 注入片段上限（字节），防用户塞超长内容撑爆请求。
const memoryInjectionMaxBytes = 4096

// SetMemoryInjectionEnabled 测试用覆盖；nil 恢复 env 默认。
func SetMemoryInjectionEnabled(v *bool) {
	common.SetFeatureFlagOverride(common.FlagMemoryInjectionEnabled, common.BoolFeatureFlagOverride(v))
}

// MemoryInjectionEnabled 报告记忆注入开关状态。
func MemoryInjectionEnabled() bool {
	return common.FeatureFlagValue(common.FlagMemoryInjectionEnabled, memoryInjectionEnvDefault)
}

// BuildMemoryInjection 构建要注入的 system 片段（纯函数）。
//   - userMemory：用户配置的记忆片段（来自 UserSetting；空则不注入）。
//   - 超过上限时截断到 memoryInjectionMaxBytes（rune 边界安全）。
//
// 返回空串表示不注入。调用方据返回非空决定是否插入 system。
func BuildMemoryInjection(userMemory string) string {
	if !MemoryInjectionEnabled() {
		return ""
	}
	userMemory = strings.TrimSpace(userMemory)
	if userMemory == "" {
		return ""
	}
	return truncateOnRuneBoundary(userMemory, memoryInjectionMaxBytes)
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

// BuildUserContextBlock T13：把「记忆 + 启用中的技能」拼成一段稳定的 system 前缀。
//
// 顺序固定（记忆在前、技能按名称升序在后）——稳定性是为了让上游前缀缓存命中：
// 同一用户的每次请求这段前缀完全一致，只有对话本体变化。
//
// 复用同一个开关（MEMORY_INJECTION_ENABLED）与同一套长度上限：技能与记忆本质
// 是同一种东西（用户自定义的 system 片段），没有必要引入第二个开关让运维困惑。
// 开关关闭时返回空串（零行为变化）。
func BuildUserContextBlock(memory string, skills []dto.UserSkill) string {
	if !MemoryInjectionEnabled() {
		return ""
	}
	block := BuildMemoryInjection(memory)
	skillBlock := buildSkillBlock(skills)
	switch {
	case block == "":
		return skillBlock
	case skillBlock == "":
		return block
	default:
		return block + "\n" + skillBlock
	}
}

// buildSkillBlock 把启用中的技能渲染为一段文本（纯函数）。
// 未启用/空名/空内容的技能被跳过；整体不超过 memoryInjectionMaxBytes。
func buildSkillBlock(skills []dto.UserSkill) string {
	if len(skills) == 0 {
		return ""
	}
	// 按名称排序保证前缀稳定（用户重排顺序不应改变上游看到的文本）。
	active := make([]dto.UserSkill, 0, len(skills))
	for _, s := range skills {
		if !s.Enabled {
			continue
		}
		name := strings.TrimSpace(s.Name)
		prompt := strings.TrimSpace(s.Prompt)
		if name == "" || prompt == "" {
			continue
		}
		active = append(active, dto.UserSkill{Name: name, Prompt: prompt})
	}
	if len(active) == 0 {
		return ""
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Name < active[j].Name })

	var sb strings.Builder
	sb.WriteString(buildSkillHeader)
	for _, s := range active {
		sb.WriteString("\n- ")
		sb.WriteString(s.Name)
		sb.WriteString(": ")
		sb.WriteString(s.Prompt)
		if sb.Len() > memoryInjectionMaxBytes {
			return truncateOnRuneBoundary(sb.String(), memoryInjectionMaxBytes)
		}
	}
	return sb.String()
}

// buildSkillHeader 技能块的引导语。用英文以便与各上游模型的指令理解保持一致；
// 具体技能内容由用户自定义（可中文）。
const buildSkillHeader = "Follow these user-defined skills when relevant:"

// truncateOnRuneBoundary 截断到不超过 maxBytes，且不切裂 UTF-8 字符。
//
// 用 DecodeLastRune 判定尾部是否为**不完整的**多字节序列（此时返回 RuneError+size 1），
// 逐字节回退到最后一个完整 rune 的结束位置。仅检查末字节的高位（旧写法）不够：
// 它无法区分「合法 rune 的续字节」与「被切掉前缀的残缺序列」，对中文会产出非法 UTF-8。
func truncateOnRuneBoundary(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	b := []byte(s)[:maxBytes]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		if r != utf8.RuneError || size > 1 {
			break
		}
		b = b[:len(b)-1]
	}
	return string(b)
}
