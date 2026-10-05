package model

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §B1-2 迁移：开启后把明文 key 加密（幂等），关闭时不动作。
func TestMigrateChannelKeyEncryption(t *testing.T) {
	truncateTables(t)
	common.CryptoSecret = "migration-test-secret"

	// 关闭开关 → no-op（明文保持）。
	common.ChannelKeyEncryptionEnabled = false
	require.NoError(t, DB.Create(&Channel{Name: "c-off", Key: "plain-key", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, migrateChannelKeyEncryption())
	var off Channel
	require.NoError(t, DB.Where("name = ?", "c-off").First(&off).Error)
	// AfterFind 未解密（开关关），DB 值即明文。
	var rawOff string
	require.NoError(t, DB.Model(&Channel{}).Where("name = ?", "c-off").Select("key").Scan(&rawOff).Error)
	assert.Equal(t, "plain-key", rawOff, "flag off must leave plaintext")

	// 开启开关 → 加密落库；读取自动解密。
	t.Cleanup(func() { common.ChannelKeyEncryptionEnabled = false })
	require.NoError(t, DB.Create(&Channel{Name: "c-on", Key: "secret-key-1", Status: common.ChannelStatusEnabled}).Error)
	common.ChannelKeyEncryptionEnabled = true
	require.NoError(t, migrateChannelKeyEncryption())

	var rawOn string
	require.NoError(t, DB.Model(&Channel{}).Where("name = ?", "c-on").Select("key").Scan(&rawOn).Error)
	assert.True(t, common.IsChannelKeyEncrypted(rawOn), "migrated value must be ciphertext")
	assert.NotContains(t, rawOn, "secret-key-1")

	// 幂等：再跑一次不改变（已加密跳过）。
	require.NoError(t, migrateChannelKeyEncryption())
	var rawOn2 string
	require.NoError(t, DB.Model(&Channel{}).Where("name = ?", "c-on").Select("key").Scan(&rawOn2).Error)
	assert.Equal(t, rawOn, rawOn2, "second run must be idempotent")
}
