package common

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 底层实现细节不外泄（尼尔森 H9.1 / N15）：
// 用户不该看到 `sql: no rows in result set` 这类串，但它必须留在服务端日志里。
func TestLooksInternalError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"sql: no rows in result set", true},
		{"Error 1062: Duplicate entry '1' for key 'PRIMARY'", true},
		{"UNIQUE constraint failed: users.username", true},
		{"json: cannot unmarshal number into Go struct field", true},
		{"runtime error: invalid memory address or nil pointer dereference", true},
		{"context deadline exceeded", true},
		{"额度不足，请充值", false},
		{"channel not found", false},
		{"Invalid API key provided", false},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.want, LooksInternalError(tc.msg), "msg=%q", tc.msg)
	}
}

func newTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	return ctx, recorder
}

func TestApiErrorSanitizesInternalErrors(t *testing.T) {
	ctx, recorder := newTestContext(t)

	ApiError(ctx, errors.New("sql: no rows in result set"))

	body := recorder.Body.String()
	require.Contains(t, body, `"success":false`)
	require.Contains(t, body, `"code":"internal_error"`)
	require.Contains(t, body, `"request_id":"`)
	require.NotContains(t, body, "no rows in result set", "底层错误不得原样返回")
	// 同一个 request id 同时出现在响应头与 body，用户报障时可直接对照服务端日志。
	require.NotEmpty(t, ctx.Writer.Header().Get(RequestIdKey))
}

func TestApiErrorPassesThroughUserFacingMessages(t *testing.T) {
	ctx, recorder := newTestContext(t)

	ApiError(ctx, errors.New("额度不足，请充值"))

	body := recorder.Body.String()
	assert.True(t, strings.Contains(body, "额度不足"), "面向用户的文案必须原样保留：%s", body)
	assert.NotContains(t, body, "internal_error")
}

// 自定义提示语 + 底层细节分离（N15 收口）：
// 39 处 `ApiErrorMsg(c, "前缀: "+err.Error())` 会把驱动细节送进界面。
// ApiErrorMsgLogged 让用户只看到人话，细节进服务端日志，两者共用一个 request id。
func TestApiErrorMsgLoggedHidesDetailsButKeepsUserMessage(t *testing.T) {
	ctx, recorder := newTestContext(t)

	ApiErrorMsgLogged(ctx, "解析 Discovery 配置失败", errors.New("json: cannot unmarshal string into Go struct field"))

	body := recorder.Body.String()
	require.Contains(t, body, `"success":false`)
	require.Contains(t, body, "解析 Discovery 配置失败", "调用方给定的文案必须原样返回")
	require.NotContains(t, body, "cannot unmarshal", "底层错误不得进入响应体")
	require.NotContains(t, body, "json:", "底层错误不得进入响应体")
	// 日志侧与响应侧共享同一个 request id，便于用户报障时对照。
	require.NotEmpty(t, ctx.Writer.Header().Get(RequestIdKey))
}

func TestApiErrorMsgLoggedToleratesNilError(t *testing.T) {
	ctx, recorder := newTestContext(t)

	ApiErrorMsgLogged(ctx, "参数不合法", nil)

	body := recorder.Body.String()
	require.Contains(t, body, "参数不合法")
	require.NotContains(t, body, `"code"`, "固定文案路径不应附带 internal_error 码")
}
