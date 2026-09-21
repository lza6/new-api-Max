package service

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func comboForTest(t *testing.T, name, strategy string, sticky int, items []model.ComboModelItem) *model.ChannelCombo {
	t.Helper()
	b, err := common.Marshal(items)
	require.NoError(t, err)
	return &model.ChannelCombo{Id: 1, Name: name, Strategy: strategy, Sticky: sticky, Models: string(b)}
}

func TestComboFallbackReturnsFirst(t *testing.T) {
	combo := comboForTest(t, "fb", "fallback", 0, []model.ComboModelItem{
		{ChannelID: 1, Model: "m1"}, {ChannelID: 2, Model: "m2"},
	})
	cand, err := ComboNextCandidate(combo)
	require.NoError(t, err)
	assert.Equal(t, 1, cand.ChannelID)
	assert.Equal(t, "m1", cand.Model)
}

func TestComboRoundRobinRotatesWithSticky(t *testing.T) {
	combo := comboForTest(t, "rr", "round-robin", 2, []model.ComboModelItem{
		{ChannelID: 1, Model: "m1"}, {ChannelID: 2, Model: "m2"},
	})
	// 连续两次应返回同一候选（sticky=2）。
	first, err := ComboNextCandidate(combo)
	require.NoError(t, err)
	second, err := ComboNextCandidate(combo)
	require.NoError(t, err)
	assert.Equal(t, first.ChannelID, second.ChannelID)
	// 第三次轮换。
	third, err := ComboNextCandidate(combo)
	require.NoError(t, err)
	assert.NotEqual(t, first.ChannelID, third.ChannelID)
}

func TestComboWeightedPick(t *testing.T) {
	combo := comboForTest(t, "wt", "weighted", 0, []model.ComboModelItem{
		{ChannelID: 1, Model: "m1", Weight: 90}, {ChannelID: 2, Model: "m2", Weight: 10},
	})
	seen := map[int]bool{}
	// 200 次抽样：10% 权重候选至少出现一次的理论失败率从 ~12%（20 次）
	// 降至 ~7e-10，消除随机抽样导致的间歇性误报。
	for range 200 {
		cand, err := ComboNextCandidate(combo)
		require.NoError(t, err)
		seen[cand.ChannelID] = true
	}
	assert.True(t, seen[1], "高权重候选应至少出现一次")
	assert.True(t, seen[2], "低权重候选也至少出现一次")
}

func TestComboFailAndAdvance(t *testing.T) {
	combo := comboForTest(t, "fba", "fallback", 0, []model.ComboModelItem{
		{ChannelID: 1, Model: "m1"}, {ChannelID: 2, Model: "m2"}, {ChannelID: 3, Model: "m3"},
	})
	// 候选失败推进一次：fallback 首个应推进到 m2。
	ComboFailAndAdvance(combo)
	cand, err := ComboNextCandidate(combo)
	require.NoError(t, err)
	assert.Equal(t, 2, cand.ChannelID)
	assert.Equal(t, "m2", cand.Model)
}
