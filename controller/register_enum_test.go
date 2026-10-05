package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// freshRegisterTestDB 独立内存库 + 开启注册/邮箱验证。
func freshRegisterTestDB(t *testing.T) {
	t.Helper()
	prevDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB = db
	prevReg, prevPwd, prevEmail := common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled
	t.Cleanup(func() {
		model.DB = prevDB
		common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = prevReg, prevPwd, prevEmail
	})
	common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = true, true, true
}

func doRegister(t *testing.T, email string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// 预先写入验证码（purpose 与注册一致）。
	common.RegisterVerificationCodeWithKey(email, "123456", common.EmailVerificationPurpose)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/user/register",
		strings.NewReader(`{"username":"u_`+strings.Split(email, "@")[0]+`","password":"Passw0rd!","email":"`+email+`","verification_code":"123456"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	Register(c)
	return rec
}

// §B1-1 邮箱防枚举：已占用邮箱注册 → 200 + {"success":true}（与成功不可区分），
// 既不返回可区分错误码，也不泄露「邮箱已注册」。
func TestRegisterEmailTakenReturnsUniformSuccess(t *testing.T) {
	freshRegisterTestDB(t)
	// 预置已占用邮箱。
	require.NoError(t, model.DB.Create(&model.User{
		Username: "existing", Email: "taken@example.com", Password: "x",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "aff1",
	}).Error)

	rec := doRegister(t, "taken@example.com")
	require.Equal(t, 200, rec.Code, "email-taken must NOT return an error status")
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"], "must return success shape")
	// 不含任何暗示邮箱已存在的业务错误码。
	_, hasErrCode := body["code"]
	assert.False(t, hasErrCode, "no distinguishable error code")

	// 断言：DB 未新增该邮箱用户（确实没注册成功）。
	var n int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("email = ?", "taken@example.com").Count(&n).Error)
	assert.EqualValues(t, 1, n, "must not create a duplicate user")
}

// 对照：未占用邮箱注册 → 200 成功（响应形状与上面一致）。
func TestRegisterFreshEmailAlso200(t *testing.T) {
	freshRegisterTestDB(t)
	rec := doRegister(t, "fresh@example.com")
	require.Equal(t, 200, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
}
