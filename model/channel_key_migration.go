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
//   - 回滚：关闭开关后读取仍 fail-open（无前缀按明文）；如需真正回退成明文，
//     需管理员导出后用旧值覆盖（本函数不提供解密回写，避免误操作）。
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
