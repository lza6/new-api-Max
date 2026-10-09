package openai

import (
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/constant"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRelayInfo 构造一个最小的可用 RelayInfo（含 ChannelMeta，
// ConvertOpenAIRequest 会解引用它，不能只给一个裸结构体）。
func newTestRelayInfo(setting dto.UserSetting) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "deepseek-v4.1-flash",
		},
		UserSetting: setting,
	}
}

// T13 Skills 注入的**端到端接线测试**（请求构造层，不是纯函数层）。
//
// 覆盖三件事，缺一不可：
//  1. 启用中的技能真的进入发往上游的 messages（不是只存在于 UserSetting）；
//  2. 未启用的技能不进入；
//  3. 开关关闭时 messages 逐字节不变（零行为变化）。
//
// 用真实的 adaptor.ConvertOpenAIRequest 路径，而不是直接调 injectUserMemory，
// 否则证明不了「生产路径调用了它」。
func TestSkillsInjectionReachesUpstreamMessages(t *testing.T) {
	on := true
	service.SetMemoryInjectionEnabled(&on)
	defer service.SetMemoryInjectionEnabled(nil)

	a := &Adaptor{}
	info := newTestRelayInfo(dto.UserSetting{
		MemoryInjection: "我常用 Go",
		Skills: []dto.UserSkill{
			{Name: "concise", Prompt: "Answer in one paragraph first", Enabled: true},
			{Name: "ignored", Prompt: "SHOULD NOT APPEAR", Enabled: false},
		},
	})

	req := &dto.GeneralOpenAIRequest{
		Model:    "deepseek-v4.1-flash",
		Messages: []dto.Message{{Role: "user", Content: "hi"}},
	}

	out, err := a.ConvertOpenAIRequest(nil, info, req)
	require.NoError(t, err)
	converted, ok := out.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	require.NotEmpty(t, converted.Messages)
	assert.Equal(t, "system", converted.Messages[0].Role, "应注入一条 system 消息")

	system, _ := converted.Messages[0].Content.(string)
	assert.Contains(t, system, "我常用 Go", "记忆必须进入上游请求")
	assert.Contains(t, system, "concise", "启用中的技能必须进入上游请求")
	assert.Contains(t, system, "Answer in one paragraph first")
	assert.NotContains(t, system, "SHOULD NOT APPEAR", "未启用的技能不得进入上游请求")

	// 原有用户消息必须原样保留。
	require.Len(t, converted.Messages, 2)
	assert.Equal(t, "hi", converted.Messages[1].Content)
}

// 已有 system 消息时：注入块拼在**其前**（稳定前缀利于上游缓存），不新增消息。
func TestSkillsInjectionMergesIntoExistingSystem(t *testing.T) {
	on := true
	service.SetMemoryInjectionEnabled(&on)
	defer service.SetMemoryInjectionEnabled(nil)

	a := &Adaptor{}
	info := newTestRelayInfo(dto.UserSetting{
		Skills: []dto.UserSkill{{Name: "s", Prompt: "skill body", Enabled: true}},
	})

	req := &dto.GeneralOpenAIRequest{
		Model: "deepseek-v4.1-flash",
		Messages: []dto.Message{
			{Role: "system", Content: "you are helpful"},
			{Role: "user", Content: "hi"},
		},
	}

	out, err := a.ConvertOpenAIRequest(nil, info, req)
	require.NoError(t, err)
	converted := out.(*dto.GeneralOpenAIRequest)

	require.Len(t, converted.Messages, 2, "不应新增消息")
	system, _ := converted.Messages[0].Content.(string)
	assert.True(t, strings.HasPrefix(system, "Follow these user-defined skills"),
		"注入块必须在原 system 之前，实际: %q", system)
	assert.True(t, strings.HasSuffix(system, "you are helpful"))
}

// 开关关闭时：即便配了技能与记忆，messages 也必须逐字节不变。
func TestSkillsInjectionDisabledLeavesMessagesUntouched(t *testing.T) {
	off := false
	service.SetMemoryInjectionEnabled(&off)
	defer service.SetMemoryInjectionEnabled(nil)

	a := &Adaptor{}
	info := newTestRelayInfo(dto.UserSetting{
		MemoryInjection: "mem",
		Skills:          []dto.UserSkill{{Name: "s", Prompt: "p", Enabled: true}},
	})

	req := &dto.GeneralOpenAIRequest{
		Model:    "deepseek-v4.1-flash",
		Messages: []dto.Message{{Role: "user", Content: "hi"}},
	}

	out, err := a.ConvertOpenAIRequest(nil, info, req)
	require.NoError(t, err)
	converted := out.(*dto.GeneralOpenAIRequest)

	require.Len(t, converted.Messages, 1, "开关关闭时不得新增消息")
	assert.Equal(t, "user", converted.Messages[0].Role)
	assert.Equal(t, "hi", converted.Messages[0].Content)
}
