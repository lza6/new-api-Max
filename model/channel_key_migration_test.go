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

// §B1-2 缺陷回归：单列直写 Update("key", ...) 不触发 BeforeSave，
// 会让 Codex OAuth 令牌刷新等路径把明文写进库、静默绕过加密。
// UpdateChannelKeyColumn 是这类写入的唯一正确入口。
func TestUpdateChannelKeyColumnEncryptsWhenEnabled(t *testing.T) {
	truncateTables(t)
	oldSecret := common.CryptoSecret
	oldEnabled := common.ChannelKeyEncryptionEnabled
	common.CryptoSecret = "direct-update-test-secret"
	t.Cleanup(func() {
		common.CryptoSecret = oldSecret
		common.ChannelKeyEncryptionEnabled = oldEnabled
	})

	ch := Channel{Name: "codex-refresh-target", Key: "old-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(&ch).Error)

	common.ChannelKeyEncryptionEnabled = true
	require.NoError(t, UpdateChannelKeyColumn(ch.Id, "PLAIN-REFRESHED-TOKEN"))

	var raw string
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", ch.Id).Select("key").Scan(&raw).Error)
	assert.True(t, common.IsChannelKeyEncrypted(raw),
		"direct key column update must be encrypted, got %q", raw)
	assert.NotContains(t, raw, "PLAIN-REFRESHED-TOKEN")

	// 读回路径必须能解出原文（AfterFind）。
	var loaded Channel
	require.NoError(t, DB.Where("id = ?", ch.Id).First(&loaded).Error)
	assert.Equal(t, "PLAIN-REFRESHED-TOKEN", loaded.Key)
}

// 开关关闭时应保持明文写入（零行为变化），且失败校验 channelId。
func TestUpdateChannelKeyColumnRespectsDisabledFlag(t *testing.T) {
	truncateTables(t)
	oldEnabled := common.ChannelKeyEncryptionEnabled
	common.ChannelKeyEncryptionEnabled = false
	t.Cleanup(func() { common.ChannelKeyEncryptionEnabled = oldEnabled })

	ch := Channel{Name: "plain-target", Key: "old-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(&ch).Error)

	require.NoError(t, UpdateChannelKeyColumn(ch.Id, "plain-refreshed"))
	var raw string
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", ch.Id).Select("key").Scan(&raw).Error)
	assert.Equal(t, "plain-refreshed", raw)

	assert.Error(t, UpdateChannelKeyColumn(0, "x"), "zero channel id must be rejected")
}

// §B1-2 启动自检：开关关闭但库中已有密文 → 必须告警（检测危险态）。
func TestAssertChannelKeyEncryptionStateDetectsMismatch(t *testing.T) {
	truncateTables(t)
	oldSecret := common.CryptoSecret
	oldEnabled := common.ChannelKeyEncryptionEnabled
	common.CryptoSecret = "state-check-secret"
	t.Cleanup(func() {
		common.CryptoSecret = oldSecret
		common.ChannelKeyEncryptionEnabled = oldEnabled
	})

	// 开启写入一条密文，再关掉开关模拟「开关与库内容不一致」。
	common.ChannelKeyEncryptionEnabled = true
	require.NoError(t, DB.Create(&Channel{Name: "enc-row", Key: "secret", Status: common.ChannelStatusEnabled}).Error)
	var raw string
	require.NoError(t, DB.Model(&Channel{}).Where("name = ?", "enc-row").Select("key").Scan(&raw).Error)
	require.True(t, common.IsChannelKeyEncrypted(raw), "precondition: row must be ciphertext")

	common.ChannelKeyEncryptionEnabled = false
	// 只要求不 panic 且能正确识别；告警走 SysError 日志。
	assert.NotPanics(t, func() { AssertChannelKeyEncryptionState() })

	var count int64
	require.NoError(t, DB.Model(&Channel{}).
		Where("key LIKE ?", common.ChannelKeyEncryptedPrefixPattern()).
		Count(&count).Error)
	assert.EqualValues(t, 1, count, "detection query must find the ciphertext row")
}
