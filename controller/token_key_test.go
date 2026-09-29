package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenKeyDisclosureOwnershipAndStatus 安全回归（2026-09-29）：
// 查看/批量查看 API 密钥在「免二次验证」之后，归属校验成为唯一安全边界。
// 本用例锁定三条契约：
//  1. 读自己的密钥 → 200 且返回明文（验证免 step-up 生效）
//  2. 读他人的密钥 → 404 + TOKEN_NOT_FOUND（既拒绝，又不用 200+错误文本
//     误导调用方、也不给 id 是否存在提供枚举信号）
//  3. 批量：全越权 → 404；部分越权 → 200 只返回自己的子集（不得泄露他人）
func TestTokenKeyDisclosureOwnershipAndStatus(t *testing.T) {
	require.NoError(t, i18n.Init())
	model.InitCol() // 复位 commonKeyCol（批量查询用），否则测试库 SQL 语法错
	db := setupTokenControllerTestDB(t)
	owner := seedToken(t, db, 1, "owner-key", "owner-raw-key-0000000000000000000000000")
	other := seedToken(t, db, 2, "other-secret", "other-raw-key-0000000000000000000000000")

	// 1) 读自己的 → 200 + 明文
	ctx, rec := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(owner.Id)+"/key", nil, 1)
	ctx.Params = ginParams("id", strconv.Itoa(owner.Id))
	GetTokenKey(ctx)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "owner-raw-key")

	// 2) 读他人的 → 404 + 稳定错误码，且不泄露对方的 key
	ctx2, rec2 := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(other.Id)+"/key", nil, 1)
	ctx2.Params = ginParams("id", strconv.Itoa(other.Id))
	GetTokenKey(ctx2)
	assert.Equal(t, http.StatusNotFound, rec2.Code, rec2.Body.String())
	assert.Contains(t, rec2.Body.String(), "TOKEN_NOT_FOUND")
	assert.NotContains(t, rec2.Body.String(), "other-raw-key", "越权响应绝不能带出对方密钥")

	// 3a) 批量：全越权 → 404，且响应体无任何密钥
	ctx3, rec3 := newAuthenticatedContext(t, http.MethodPost, "/api/token/batch/keys",
		map[string]any{"ids": []int{other.Id}}, 1)
	GetTokenKeysBatch(ctx3)
	assert.Equal(t, http.StatusNotFound, rec3.Code, rec3.Body.String())
	assert.NotContains(t, rec3.Body.String(), "other-raw-key")

	// 3b) 批量：混合 → 200 但只含自己的
	ctx4, rec4 := newAuthenticatedContext(t, http.MethodPost, "/api/token/batch/keys",
		map[string]any{"ids": []int{owner.Id, other.Id}}, 1)
	GetTokenKeysBatch(ctx4)
	require.Equal(t, http.StatusOK, rec4.Code, rec4.Body.String())
	assert.Contains(t, rec4.Body.String(), "owner-raw-key")
	assert.NotContains(t, rec4.Body.String(), "other-raw-key", "批量结果不得包含他人密钥")
}

// ginParams 构造 gin 路径参数（控制器直调时需要）。
func ginParams(kv ...string) gin.Params {
	params := make(gin.Params, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		params = append(params, gin.Param{Key: kv[i], Value: kv[i+1]})
	}
	return params
}

// TestTokenKeyNotFoundMessageLocalized 错误文案必须本地化（非裸 key）。
func TestTokenKeyNotFoundMessageLocalized(t *testing.T) {
	require.NoError(t, i18n.Init())
	model.InitCol() // 复位 commonKeyCol（批量查询用），否则测试库 SQL 语法错
	db := setupTokenControllerTestDB(t)
	ctx, rec := newAuthenticatedContext(t, http.MethodPost, "/api/token/999999/key", nil, 1)
	ctx.Params = ginParams("id", "999999")
	GetTokenKey(ctx)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "token.not_exists", "文案必须已翻译，不能回落到 key")
	assert.NotEmpty(t, body)
	_ = fmt.Sprint(db)
	_ = model.Token{}
}
