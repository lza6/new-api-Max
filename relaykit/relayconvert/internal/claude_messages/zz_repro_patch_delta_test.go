package claudemessages

// 复现用例：缺陷 B 的应用点 —— PatchClaudeMessageDeltaUsageData。
//
// 背景（上游 issue #7777）：PatchClaudeMessageDeltaUsageData
// （internal/claude_messages/to_oai_chat_resp.go:461-476）在 message_delta
// 只有 cache creation 总量时，会把归一化后的 5m/1h 写回流式 JSON。
// 若归一化把剩余全部归入 5m，则这里补出的 ephemeral_5m_input_tokens 会
// 等于总量、ephemeral_1h_input_tokens=0，下游据此按 5m 单价计费。
//
// 本文件只断言当前行为，不改产品代码。

import (
	"testing"

	"github.com/lza6/new-api-Max/relaykit/dto"
)

// TestRepro_PatchClaudeMessageDeltaUsageData_TotalOnlyBecomesAll5m 验证：
// message_delta 只带 cache_creation_input_tokens=2000（无拆分）时，
// BuildMessageDeltaPatchUsage + NormalizeCacheCreationSplit 会把
// 全部 2000 记为 ephemeral_5m。
func TestRepro_PatchClaudeMessageDeltaUsageData_TotalOnlyBecomesAll5m(t *testing.T) {
	// 上游 message_delta：只有总量，没有 5m/1h 拆分对象。
	delta := &dto.ClaudeResponse{
		Type: "message_delta",
		Usage: &dto.ClaudeUsage{
			CacheCreationInputTokens: 2000,
		},
	}
	info := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokensDetails: dto.InputTokenDetails{
				CachedCreationTokens: 2000,
			},
		},
	}

	patched := BuildMessageDeltaPatchUsage(delta, info)

	// 期望行为（上游已修）：未拆分总量不应伪装成「全部 5m」。
	// 当前行为（疑似缺陷）：全部 2000 归入 ephemeral_5m，1h=0。
	if patched.CacheCreation == nil {
		t.Fatalf("CURRENT BEHAVIOR: CacheCreation is nil; normalization did not synthesize a split")
	}
	if patched.CacheCreation.Ephemeral5mInputTokens != 2000 || patched.CacheCreation.Ephemeral1hInputTokens != 0 {
		t.Fatalf("CURRENT BEHAVIOR changed: got 5m=%d 1h=%d, want the recorded (buggy) 2000/0",
			patched.CacheCreation.Ephemeral5mInputTokens, patched.CacheCreation.Ephemeral1hInputTokens)
	}
}
