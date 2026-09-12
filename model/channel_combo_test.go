package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupComboTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelCombo{}, &Channel{}))
	original := DB
	DB = db
	t.Cleanup(func() { DB = original })
}

func comboModelsJSON(items []ComboModelItem) string {
	b, _ := common.Marshal(items)
	return string(b)
}

func TestCreateComboValidatesAndPersists(t *testing.T) {
	setupComboTest(t)

	// 合法创建。
	combo := &ChannelCombo{
		Name:     "fast-combo",
		Strategy: "fallback",
		Models:   comboModelsJSON([]ComboModelItem{{ChannelID: 1, Model: "glm-4.6"}}),
	}
	require.NoError(t, CreateCombo(combo))
	require.Positive(t, combo.Id)

	// 名称重复拒绝。
	dup := &ChannelCombo{Name: "fast-combo", Strategy: "fallback", Models: combo.Models}
	require.Error(t, CreateCombo(dup))

	// 空 models 拒绝。
	empty := &ChannelCombo{Name: "empty", Strategy: "fallback", Models: "[]"}
	require.Error(t, CreateCombo(empty))

	// 非法 strategy 拒绝。
	bad := &ChannelCombo{Name: "bad", Strategy: "nope", Models: combo.Models}
	require.Error(t, CreateCombo(bad))
}

func TestComboCRUDAndParse(t *testing.T) {
	setupComboTest(t)

	items := []ComboModelItem{{ChannelID: 2, Model: "claude-opus"}, {ChannelID: 3, Model: "gpt-5"}}
	combo := &ChannelCombo{Name: "c1", Strategy: "weighted", Sticky: 2, Models: comboModelsJSON(items)}
	require.NoError(t, CreateCombo(combo))

	got, err := GetEnabledComboByName("c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "weighted", got.Strategy)
	parsed, err := got.ParseComboModels()
	require.NoError(t, err)
	require.Len(t, parsed, 2)
	assert.Equal(t, "gpt-5", parsed[1].Model)

	// 禁用后不再命中。
	got.Status = 2
	require.NoError(t, UpdateCombo(got))
	disabled, err := GetEnabledComboByName("c1")
	require.NoError(t, err)
	assert.Nil(t, disabled, "禁用组合不应被命中")

	// 删除后不再命中。
	require.NoError(t, DeleteCombo(got.Id))
	deleted, err := GetEnabledComboByName("c1")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestListCombosPagination(t *testing.T) {
	setupComboTest(t)
	for i := range 5 {
		combo := &ChannelCombo{Name: "combo" + string(rune('a'+i)), Strategy: "fallback",
			Models: comboModelsJSON([]ComboModelItem{{ChannelID: 1, Model: "m"}})}
		require.NoError(t, CreateCombo(combo))
	}
	all, total, err := ListCombos(0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, all, 5)
}
