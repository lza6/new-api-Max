package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// §B1-2 渠道密钥加密（AES-256-GCM，默认关闭）。
//
// 目标：渠道 `Channel.Key` 明文存库的问题。**不新增列、不改列类型**（保三库兼容），
// 而是把密文写进原 `key` 列，以固定前缀 `enc:v1:` 区分明文/密文——读取时按前缀判断
// 是否解密。默认关闭（`CHANNEL_KEY_ENCRYPTION`）时写入/读取路径零变化（明文照常）。
//
// 主密钥：从 `CRYPTO_SECRET`（部署级密钥，跨节点一致）派生 SHA-256。
// 密文 = base64( nonce(12) || ciphertext )，AES-256-GCM 提供机密性 + 完整性。
//
// 兼容：读取时若值无 `enc:v1:` 前缀 → 视为明文直接返回（旧数据 fail-open）；
// 迁移幂等：对已加密值再次加密会被 DecryptIfEncrypted 跳过（有前缀即不再加）。
//
// ⚠️ 重要：「fail-open」只对**旧的明文数据**成立。若库中已存在 `enc:v1:` 密文，
// 却把开关 CHANNEL_KEY_ENCRYPTION 关掉，AfterFind 会直接 return，channel.Key
// 保持密文原样发往上游 → 全部渠道鉴权失败（401）。即关闭开关对已加密库**不是**
// 安全降级而是服务中断。启动时由 assertChannelKeyEncryptionState 检测并告警。

const (
	channelKeyCipherPrefix = "enc:v1:"
)

// ChannelKeyEncryptionEnabled 出站渠道密钥加密开关（env CHANNEL_KEY_ENCRYPTION，默认 false）。
// 开启后**下次保存渠道即加密**，读取自动解密；旧明文数据 fail-open（无前缀按明文）。
var ChannelKeyEncryptionEnabled = false

var ErrChannelKeyDecrypt = errors.New("channel key decrypt failed")

// deriveChannelKeyAESKey 从 CRYPTO_SECRET 派生 32 字节 AES 密钥。
func deriveChannelKeyAESKey() []byte {
	sum := sha256.Sum256([]byte("new-api:channel-key:v1:" + CryptoSecret))
	return sum[:]
}

// IsChannelKeyEncrypted 报告字符串是否为本模块产出的密文（带版本前缀）。
func IsChannelKeyEncrypted(value string) bool {
	return strings.HasPrefix(value, channelKeyCipherPrefix)
}

// ChannelKeyEncryptedPrefixPattern 返回供 SQL LIKE 使用的前缀匹配串（`enc:v1:%`）。
// 调用方用它做「库中是否存在密文」的原始列查询，避免把前缀硬编码到 model 层。
func ChannelKeyEncryptedPrefixPattern() string {
	return channelKeyCipherPrefix + "%"
}

// EncryptChannelKey 加密渠道密钥（含多 key 的整段文本亦可，按整体字节加密）。
// 已加密值原样返回（幂等）。空值原样返回。
func EncryptChannelKey(plain string) (string, error) {
	if plain == "" || IsChannelKeyEncrypted(plain) {
		return plain, nil
	}
	block, err := aes.NewCipher(deriveChannelKeyAESKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return channelKeyCipherPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// DecryptChannelKey 解密渠道密钥。非密文（无前缀）原样返回——旧明文数据 fail-open。
func DecryptChannelKey(value string) (string, error) {
	if value == "" || !IsChannelKeyEncrypted(value) {
		return value, nil
	}
	raw := strings.TrimPrefix(value, channelKeyCipherPrefix)
	sealed, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", ErrChannelKeyDecrypt
	}
	block, err := aes.NewCipher(deriveChannelKeyAESKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", ErrChannelKeyDecrypt
	}
	nonce, ct := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrChannelKeyDecrypt
	}
	return string(plain), nil
}
