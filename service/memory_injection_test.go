package service

import (
	"strings"
	"testing"

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
