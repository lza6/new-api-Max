package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §B1-2 渠道密钥加密：往返、幂等、明文兼容、多 key 文本、错误密钥。
func TestChannelKeyEncryptRoundTrip(t *testing.T) {
	CryptoSecret = "test-secret-for-channel-key"
	cases := []string{
		"sk-single-key-123",
		"sk-key1\nsk-key2\nsk-key3",
		`["vertex-key-a","vertex-key-b"]`,
	}
	for _, plain := range cases {
		enc, err := EncryptChannelKey(plain)
		require.NoError(t, err)
		assert.True(t, IsChannelKeyEncrypted(enc), "encrypted must carry prefix")
		assert.NotContains(t, enc, plain, "ciphertext must not contain plaintext")
		dec, err := DecryptChannelKey(enc)
		require.NoError(t, err)
		assert.Equal(t, plain, dec, "round-trip must restore plaintext")
	}
}

// 幂等：已加密值再加密不变；空值原样。
func TestChannelKeyEncryptIdempotent(t *testing.T) {
	CryptoSecret = "s"
	enc, _ := EncryptChannelKey("k")
	enc2, _ := EncryptChannelKey(enc)
	assert.Equal(t, enc, enc2, "already-encrypted must be returned as-is")
	empty, _ := EncryptChannelKey("")
	assert.Equal(t, "", empty)
}

// 旧明文 fail-open：无前缀按明文返回。
func TestChannelKeyDecryptPlaintextPassthrough(t *testing.T) {
	CryptoSecret = "s"
	dec, err := DecryptChannelKey("plain-legacy-key")
	require.NoError(t, err)
	assert.Equal(t, "plain-legacy-key", dec)
}

// 错误主密钥 → 解密失败（完整性由 GCM 保证）。
func TestChannelKeyDecryptWrongSecretFails(t *testing.T) {
	CryptoSecret = "secret-a"
	enc, _ := EncryptChannelKey("k")
	CryptoSecret = "secret-b"
	_, err := DecryptChannelKey(enc)
	assert.Error(t, err)
}

// 篡改密文 → 失败（GCM 完整性）。
func TestChannelKeyTamperDetected(t *testing.T) {
	CryptoSecret = "s"
	enc, _ := EncryptChannelKey("hello")
	tampered := enc[:len(enc)-2] + "AA"
	_, err := DecryptChannelKey(tampered)
	assert.Error(t, err)
}

var _ = strings.TrimSpace
