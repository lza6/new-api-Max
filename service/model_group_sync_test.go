package service

import (
	"testing"

	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
)

func TestMergeGroups(t *testing.T) {
	assert.Equal(t, []string{"default", "free"}, mergeGroups([]string{"default"}, []string{"free"}))
	assert.Equal(t, []string{"default"}, mergeGroups([]string{"default"}, []string{"default"}))
	assert.Equal(t, []string{"a", "b"}, mergeGroups([]string{"a", "b"}, []string{"b", "a"}))
	assert.Equal(t, []string{"x"}, mergeGroups([]string{"x", ""}, nil))
	assert.Equal(t, []string{"default"}, mergeGroups([]string{}, []string{"default"}))
}

func TestNormalizeGroupList(t *testing.T) {
	assert.Equal(t, []string{"free", "vip"}, normalizeGroupList([]string{" free ", "", "free", "vip"}))
	assert.Equal(t, []string{"default"}, normalizeGroupList([]string{"", "default", ""}))
}

func TestGroupsEqual(t *testing.T) {
	assert.True(t, groupsEqual([]string{"a", "b"}, []string{"a", "b"}))
	assert.False(t, groupsEqual([]string{"a", "b"}, []string{"a"}))
	assert.False(t, groupsEqual([]string{"a", "b"}, []string{"b", "a"}))
}

func TestChannelContainsModel(t *testing.T) {
	ch := &model.Channel{Models: "gpt-4o,deepseek-v4, free-m1"}
	assert.True(t, channelContainsModel(ch, "gpt-4o"))
	assert.True(t, channelContainsModel(ch, "free-m1"))
	assert.False(t, channelContainsModel(ch, "gpt-4"))
}
