package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

// T8 记忆注入：开关关时不注入。
func TestMemoryInjectionDisabled(t *testing.T) {
	off := false
	SetMemoryInjectionEnabled(&off)
	defer SetMemoryInjectionEnabled(nil)
	assert.False(t, MemoryInjectionEnabled())
	assert.Empty(t, BuildMemoryInjection("remember I like Go"))
	assert.Equal(t, "orig", MergeMemoryIntoSystem("orig", "remember I like Go"))
}

// T8 记忆注入：开关开时注入（记忆在前，原 system 在后）。
func TestMemoryInjectionEnabled(t *testing.T) {
	on := true
	SetMemoryInjectionEnabled(&on)
	defer SetMemoryInjectionEnabled(nil)

	assert.Equal(t, "my memory", BuildMemoryInjection("my memory"))
	merged := MergeMemoryIntoSystem("you are helpful", "my memory")
	assert.Equal(t, "my memory\nyou are helpful", merged)

	// 空 system → 只注入记忆。
	assert.Equal(t, "my memory", MergeMemoryIntoSystem("", "my memory"))
	// 空记忆 → 原样。
	assert.Equal(t, "you are helpful", MergeMemoryIntoSystem("you are helpful", "   "))
}

// T8 记忆注入：超长截断到上限（rune 边界安全）。
func TestMemoryInjectionTruncation(t *testing.T) {
	on := true
	SetMemoryInjectionEnabled(&on)
	defer SetMemoryInjectionEnabled(nil)

	long := strings.Repeat("a", memoryInjectionMaxBytes+500)
	out := BuildMemoryInjection(long)
	assert.LessOrEqual(t, len(out), memoryInjectionMaxBytes)
	assert.NotEmpty(t, out)
}

// T13 Skills 注入：只有启用中的技能进入前缀，且顺序稳定（利于上游前缀缓存）。
func TestBuildUserContextBlockSkills(t *testing.T) {
	on := true
	SetMemoryInjectionEnabled(&on)
	defer SetMemoryInjectionEnabled(nil)

	skills := []dto.UserSkill{
		{Name: "zeta", Prompt: "last by name", Enabled: true},
		{Name: "alpha", Prompt: "first by name", Enabled: true},
		{Name: "off", Prompt: "must not appear", Enabled: false},
		{Name: "   ", Prompt: "blank name skipped", Enabled: true},
		{Name: "blankprompt", Prompt: "   ", Enabled: true},
	}

	block := BuildUserContextBlock("my memory", skills)
	assert.Contains(t, block, "my memory")
	assert.Contains(t, block, "alpha")
	assert.Contains(t, block, "zeta")
	assert.NotContains(t, block, "must not appear", "未启用的技能不得注入")
	assert.NotContains(t, block, "blank name skipped", "空名技能不得注入")
	assert.NotContains(t, block, "blankprompt", "空内容技能不得注入")

	// 顺序稳定：即便输入顺序颠倒，输出必须一致（前缀缓存友好）。
	reordered := []dto.UserSkill{skills[1], skills[0]}
	assert.Equal(t, block, BuildUserContextBlock("my memory", reordered),
		"技能顺序变化不得改变注入文本")

	// 记忆在前、技能在后。
	assert.Less(t, strings.Index(block, "my memory"), strings.Index(block, "alpha"))
}

// T13 Skills 注入：开关关闭时零注入（即便配了技能）。
func TestBuildUserContextBlockDisabled(t *testing.T) {
	off := false
	SetMemoryInjectionEnabled(&off)
	defer SetMemoryInjectionEnabled(nil)

	block := BuildUserContextBlock("my memory", []dto.UserSkill{{Name: "a", Prompt: "b", Enabled: true}})
	assert.Empty(t, block, "开关关闭时必须零注入")
}

// T13 Skills 注入：仅技能、无记忆时也要成立。
func TestBuildUserContextBlockSkillsOnly(t *testing.T) {
	on := true
	SetMemoryInjectionEnabled(&on)
	defer SetMemoryInjectionEnabled(nil)

	block := BuildUserContextBlock("", []dto.UserSkill{{Name: "s", Prompt: "do X", Enabled: true}})
	assert.Contains(t, block, "s")
	assert.Contains(t, block, "do X")

	// 记忆与技能都为空 → 空串（调用方据此跳过注入）。
	assert.Empty(t, BuildUserContextBlock("", nil))
	assert.Empty(t, BuildUserContextBlock("   ", []dto.UserSkill{{Name: "x", Enabled: true}}))
}

// T13 Skills 注入：整体超长时按 rune 边界截断，不切裂 UTF-8。
func TestBuildSkillBlockTruncation(t *testing.T) {
	on := true
	SetMemoryInjectionEnabled(&on)
	defer SetMemoryInjectionEnabled(nil)

	long := strings.Repeat("技", memoryInjectionMaxBytes)
	block := BuildUserContextBlock("", []dto.UserSkill{{Name: "big", Prompt: long, Enabled: true}})
	assert.LessOrEqual(t, len(block), memoryInjectionMaxBytes)
	assert.True(t, utf8.ValidString(block), "截断后必须仍是合法 UTF-8")
}
