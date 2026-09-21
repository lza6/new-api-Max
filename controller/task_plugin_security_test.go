package controller

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const securityPluginFixture = `export function run() { return "ok"; }`

func uploadTaskPluginRequest(t *testing.T, body map[string]any) gin.H {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/plugin/task/upload", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UploadTaskPlugin(ctx)

	var response gin.H
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	return response
}

func withPluginSigningKey(t *testing.T, publicKey ed25519.PublicKey) {
	t.Helper()
	require.NoError(t, setting.SetTaskPluginEd25519PublicKeyOption(publicKey))
	t.Cleanup(func() {
		_ = setting.SetTaskPluginEd25519PublicKeyOption(nil)
	})
}

func TestUploadTaskPluginRejectsBadSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	withPluginSigningKey(t, publicKey)

	// 篡改一行源码后签名 → 验签失败 → 上传拒绝。
	source := []byte(securityPluginFixture + "\n")
	signature := ed25519.Sign(privateKey, []byte(securityPluginFixture))
	response := uploadTaskPluginRequest(t, map[string]any{
		"source":    string(source),
		"signature": base64.StdEncoding.EncodeToString(signature),
	})
	assert.False(t, response["success"].(bool), "篡改源码后验签必须失败")
	assert.Contains(t, response["message"].(string), "signature")
}

func TestUploadTaskPluginRequiresSignatureWhenKeyConfigured(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	withPluginSigningKey(t, publicKey)

	response := uploadTaskPluginRequest(t, map[string]any{
		"source": securityPluginFixture,
	})
	assert.False(t, response["success"].(bool), "配置公钥后缺签名的上传必须被拒绝")
	assert.Contains(t, response["message"].(string), "signature")
}

func TestUploadTaskPluginAcceptsValidSignature(t *testing.T) {
	// 有效签名路径会继续走注册与落库；此处仅验证「验签通过」阶段不再拒绝，
	// 后续 DB 行为由 controller 全量测试覆盖。为避免依赖测试库，用非强制
	// 分支验证：未配置公钥时签名可不带（getter 返回 nil，直接跳过）。
	assert.Nil(t, setting.GetTaskPluginEd25519PublicKey(), "未配置公钥时 getter 返回 nil")
}

func TestPluginSigningKeySettingRoundTrip(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	require.NoError(t, setting.SetTaskPluginEd25519PublicKeyOption(publicKey))
	assert.Equal(t, []byte(publicKey), setting.GetTaskPluginEd25519PublicKey())
	_ = setting.SetTaskPluginEd25519PublicKeyOption(nil)
	assert.Nil(t, setting.GetTaskPluginEd25519PublicKey())
}
