package service

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §4.7.1 去重逻辑：同用户同模型 TTL 内只写一次；不同模型立即刷新。
func TestMarkUserLastModelSeenDedup(t *testing.T) {
	ResetUserLastModelMemoryForTest()
	t.Cleanup(ResetUserLastModelMemoryForTest)

	assert.True(t, markUserLastModelSeen(1, "gpt-x"), "first write must proceed")
	assert.False(t, markUserLastModelSeen(1, "gpt-x"), "same model within TTL must be deduped")
	assert.True(t, markUserLastModelSeen(1, "gpt-y"), "different model must proceed")
	assert.True(t, markUserLastModelSeen(2, "gpt-x"), "different user must proceed")
}

// 有界：超过上限后清空，不无界增长。
func TestMarkUserLastModelSeenBounded(t *testing.T) {
	ResetUserLastModelMemoryForTest()
	t.Cleanup(ResetUserLastModelMemoryForTest)
	for i := 0; i < userLastModelMemoryCap+10; i++ {
		markUserLastModelSeen(i+1, "m")
	}
	userLastModelMu.Lock()
	n := len(userLastModelSeen)
	userLastModelMu.Unlock()
	assert.LessOrEqual(t, n, userLastModelMemoryCap, "dedup table must stay bounded")
}

// UserSetting 的 last_used_model 字段可 JSON 往返（无需迁移的 JSON 列新增字段）。
func TestUserSettingLastUsedModelRoundTrip(t *testing.T) {
	raw := []byte(`{"last_used_model":"deepseek-v4-flash","language":"zh"}`)
	var s dto.UserSetting
	require.NoError(t, common.Unmarshal(raw, &s))
	assert.Equal(t, "deepseek-v4-flash", s.LastUsedModel)
	assert.Equal(t, "zh", s.Language)
}
