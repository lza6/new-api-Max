package model

import (
	"strconv"

	"github.com/lza6/new-api-Max/common"
)

// §B1-2 渠道密钥加密迁移（幂等、可回滚）。
//
// 语义：开启 CHANNEL_KEY_ENCRYPTION 后，调用一次即把所有**明文**渠道 Key 加密落库。
//   - 幂等：已加密（enc:v1: 前缀）的行跳过；重复运行无副作用。
//   - 非破坏性：仅加密，不改列类型、不删数据；解密能力内建（AfterFind）。
//   - 回滚：⚠️ **关闭开关不等于回滚**。旧明文行无前缀会被 fail-open 当明文读回，
//     但**已加密行**在关闭开关后 AfterFind 直接 return，channel.Key 保持密文发往
//     上游 → 全部渠道 401。真正回退需在**保持开关开启**的前提下用原明文覆盖
//     （本函数不提供解密回写，避免误操作）；或补齐解密 CLI/管理端点后再迁。
//     启动时 AssertChannelKeyEncryptionState 会检测该危险态并告警。
//
// 通过读取原始 key 列 + 显式加密回写完成迁移（不触发 GORM 钩子，避免二次加密）。
func migrateChannelKeyEncryption() error {
	if !common.ChannelKeyEncryptionEnabled {
		return nil
	}
	// 逐行读**原始 key 列**（Model+Select+Scan 不触发 AfterFind 解密），
	// 判断是否已加密，再对明文行显式加密写回。避免 Find(&Channel{}) 触发
	// AfterFind 解密后误判为明文而二次加密。
	type rawRow struct {
		Id  int
		Key string
	}
	var rows []rawRow
	if err := DB.Model(&Channel{}).Select("id", "key").Scan(&rows).Error; err != nil {
		return err
	}
	migrated := 0
	for _, r := range rows {
		if r.Key == "" || common.IsChannelKeyEncrypted(r.Key) {
			continue
		}
		enc, err := common.EncryptChannelKey(r.Key)
		if err != nil {
			common.SysError("failed to encrypt channel key id=" + strconv.Itoa(r.Id) + ": " + err.Error())
			continue
		}
		if err := DB.Model(&Channel{}).Where("id = ?", r.Id).Update("key", enc).Error; err != nil {
			common.SysError("failed to persist encrypted channel key id=" + strconv.Itoa(r.Id) + ": " + err.Error())
			continue
		}
		migrated++
	}
	if migrated > 0 {
		common.SysLog("channel key encryption migration: encrypted " + strconv.Itoa(migrated) + " channels")
	}
	return nil
}

// MigrateChannelKeyEncryption 供启动流程/管理端调用；未开启开关时为 no-op。
func MigrateChannelKeyEncryption() error {
	return migrateChannelKeyEncryption()
}

// AssertChannelKeyEncryptionState 启动自检：检测「开关与库内容不一致」的危险状态。
//
// 已知危险态：库中已有 `enc:v1:` 密文，但开关被关掉。此时 AfterFind 直接 return，
// channel.Key 保持密文发往上游 → 全部渠道鉴权失败（401）。这是**服务中断**而非
// 安全降级，且没有任何报错，极难排查。
//
// 本函数只告警、不改行为（不自动开/关、不改数据），保证零副作用；由启动流程调用。
func AssertChannelKeyEncryptionState() {
	if DB == nil {
		return
	}
	var encryptedCount int64
	// 原始列查询，不触发 AfterFind。
	if err := DB.Model(&Channel{}).Where("key LIKE ?", common.ChannelKeyEncryptedPrefixPattern()).Count(&encryptedCount).Error; err != nil {
		return
	}
	if encryptedCount == 0 {
		return
	}
	if !common.ChannelKeyEncryptionEnabled {
		common.SysError("channel key state mismatch: " +
			strconv.FormatInt(encryptedCount, 10) +
			" encrypted channel keys found in DB but CHANNEL_KEY_ENCRYPTION is disabled; " +
			"channels will send ciphertext upstream and fail authentication. " +
			"Set CHANNEL_KEY_ENCRYPTION=true (with the same CRYPTO_SECRET used to encrypt) to restore.")
		return
	}
	if common.CryptoSecret == "" || common.CryptoSecret == common.SessionSecret {
		common.SysError("channel key encryption is enabled but CRYPTO_SECRET is not explicitly set " +
			"(falling back to SESSION_SECRET). If SESSION_SECRET is also unset, the per-process random " +
			"secret changes on every restart and all encrypted channel keys become undecryptable.")
	}
}
