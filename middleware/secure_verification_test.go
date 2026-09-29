package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOwnTokenKeyReadSkipsStepUpByDefault 生产回归（2026-09-29）：
// 用户查看**自己的**密钥默认不再要求 step-up。安全边界是控制器层的归属校验
// （GetTokenByIds(id, userId)）；用户已通过 session 登录，二次验证属重复校验
// （生产曾收到「复制密钥被要求验证」投诉）。
// 站点把 require_verification_to_read_own_key 设为 true 时恢复旧行为。
func TestOwnTokenKeyReadSkipsStepUpByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := operation_setting.GetTokenSetting()
	previous := setting.RequireVerificationToReadOwnKey
	t.Cleanup(func() { setting.RequireVerificationToReadOwnKey = previous })

	newRouter := func() *gin.Engine {
		router := gin.New()
		router.POST("/api/token/:id/key", SecureTokenKeyVerificationRequired(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
		router.POST("/api/token/batch/keys", SecureTokenKeysBatchVerificationRequired(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
		return router
	}

	t.Run("single key: no proof required by default", func(t *testing.T) {
		setting.RequireVerificationToReadOwnKey = false
		request := httptest.NewRequest(http.MethodPost, "/api/token/7/key", nil)
		result := httptest.NewRecorder()
		newRouter().ServeHTTP(result, request)
		assert.Equal(t, http.StatusNoContent, result.Code)
	})

	t.Run("single key: proof required when enabled", func(t *testing.T) {
		setting.RequireVerificationToReadOwnKey = true
		request := httptest.NewRequest(http.MethodPost, "/api/token/7/key", nil)
		result := httptest.NewRecorder()
		newRouter().ServeHTTP(result, request)
		// 未提供 proof（且无 session）时被拦下并返回安全证明错误码族。
		require.Equal(t, http.StatusForbidden, result.Code)
		assert.Contains(t, result.Body.String(), "SECURITY_PROOF_")
		assert.NotContains(t, result.Body.String(), `"success":true`)
	})

	t.Run("batch keys: no proof required by default", func(t *testing.T) {
		setting.RequireVerificationToReadOwnKey = false
		body := `{"ids":[7,8]}`
		request := httptest.NewRequest(http.MethodPost, "/api/token/batch/keys", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		newRouter().ServeHTTP(result, request)
		assert.Equal(t, http.StatusNoContent, result.Code)
	})

	t.Run("batch keys: invalid payload still rejected", func(t *testing.T) {
		setting.RequireVerificationToReadOwnKey = false
		for _, body := range []string{`{"ids":[]}`, `{"ids":[1,1]}`, `{"ids":[0]}`, `{}`} {
			request := httptest.NewRequest(http.MethodPost, "/api/token/batch/keys", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			newRouter().ServeHTTP(result, request)
			assert.Equal(t, http.StatusBadRequest, result.Code, body)
		}
	})
}

// TestChannelKeyReadSkipsStepUpByDefault 管理端查看渠道密钥默认不要求 step-up
// （RootAuth + ChannelSensitiveWrite 已把关）。
func TestChannelKeyReadSkipsStepUpByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := operation_setting.GetTokenSetting()
	previous := setting.RequireVerificationToReadChannelKey
	t.Cleanup(func() { setting.RequireVerificationToReadChannelKey = previous })

	router := gin.New()
	router.POST("/api/channel/:id/key", SecureVerificationRequired(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	setting.RequireVerificationToReadChannelKey = false
	request := httptest.NewRequest(http.MethodPost, "/api/channel/123/key", nil)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	assert.Equal(t, http.StatusNoContent, result.Code, "默认不要求验证")

	setting.RequireVerificationToReadChannelKey = true
	request = httptest.NewRequest(http.MethodPost, "/api/channel/123/key", nil)
	result = httptest.NewRecorder()
	router.ServeHTTP(result, request)
	assert.Equal(t, http.StatusForbidden, result.Code, "开关开启时恢复验证")
}
