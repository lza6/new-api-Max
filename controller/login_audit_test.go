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
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// §4.11.3：默认配置下登录限流必须开启（暴力破解防护）。
func TestLoginRateLimitEnabledByDefault(t *testing.T) {
	assert.True(t, operation_setting.IsLoginRateLimitEnabled(),
		"login rate limit must be enabled by default (OWASP brute-force resistance)")
	assert.Equal(t, 20, operation_setting.GetLoginRateLimitNum())
	assert.Equal(t, int64(1200), operation_setting.GetLoginRateLimitDuration())
}

// useIsolatedAuditDB 把 model.LOG_DB/model.DB 指向独立内存库（含 audit_logs/users），
// 返回该库以便断言。审计类测试必须自持数据库，避免受全量测试执行顺序影响
// （与 auth_session_test.go 同一隔离策略；共享 LOG_DB 会在其它用例改动后被污染）。
func useIsolatedAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevDB, prevLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}, &model.User{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLogDB
		_ = sqlDB.Close()
	})
	return db
}

func auditLoginRows(t *testing.T, db *gorm.DB) []model.AuditLog {
	t.Helper()
	var rows []model.AuditLog
	require.NoError(t, db.Where("category = ?", model.AuditCategoryLogin).
		Order("id asc").Find(&rows).Error)
	return rows
}

// §4.11.3：失败登录必须留下审计记录（Success=false，去敏，不含密码）。
func TestLoginFailureIsAudited(t *testing.T) {
	db := useIsolatedAuditDB(t)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/user/login", nil)
	c.Request.Header.Set("User-Agent", "test-agent")
	c.Set("login_method", "password")

	// 用户不存在：user 为 nil，仅按用户名关联，userId=0，reason=invalid_credentials。
	recordLoginFailureAudit(c, "ghost_user", nil, "invalid_credentials", 401)

	rows := auditLoginRows(t, db)
	require.Len(t, rows, 1)
	row := rows[0]
	assert.False(t, row.Success, "failed login must be recorded as unsuccessful")
	assert.Equal(t, 401, row.Status)
	assert.Equal(t, model.AuditCategoryLogin, row.Category)
	assert.Equal(t, "ghost_user", row.Username)
	assert.Equal(t, "password", row.Other.LoginMethod)
	// 去敏：审计内容与参数不得包含任何密码字面量。
	assert.NotContains(t, strings.ToLower(row.Content), "password")
	if row.Other.Op != nil {
		for _, v := range row.Other.Op.Params {
			assert.NotContains(t, strings.ToLower(toString(v)), "secret-password-value")
		}
	}
}

// 用户存在但密码错：失败归属到真实 userId。
func TestLoginFailureAttributesRealUser(t *testing.T) {
	db := useIsolatedAuditDB(t)

	user := model.User{Username: "audit_user_" + common.GetRandomString(5), Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/user/login", nil)

	recordLoginFailureAudit(c, "badpassword", &user, "invalid_credentials", 401)

	rows := auditLoginRows(t, db)
	require.Len(t, rows, 1)
	assert.Equal(t, user.Id, rows[0].UserId, "failure must attribute the real user id")
	assert.Equal(t, user.Role, rows[0].ActorRole)
	assert.Equal(t, user.Username, rows[0].Username)
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// §审查 C3：被禁用用户的登录尝试必须留失败审计（此前只返回 banned 文案）。
func TestLoginDisabledUserIsAudited(t *testing.T) {
	db := useIsolatedAuditDB(t)

	user := &model.User{Id: 4242, Username: "banned_user", Role: common.RoleCommonUser, Status: common.UserStatusDisabled}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/user/login", nil)
	c.Set("login_method", "password")

	setupLoginAtAuthVersion(user, user.AuthVersion, c)

	rows := auditLoginRows(t, db)
	require.Len(t, rows, 1, "disabled-user login must be audited")
	assert.False(t, rows[0].Success)
	assert.Equal(t, 403, rows[0].Status)
	assert.Equal(t, user.Id, rows[0].UserId)
	require.NotNil(t, rows[0].Other.Op)
	if raw, ok := rows[0].Other.Op.Params["reason"].(json.RawMessage); ok {
		assert.Contains(t, string(raw), "user_disabled")
	} else {
		assert.Contains(t, toString(rows[0].Other.Op.Params["reason"]), "user_disabled")
	}
}

// 反枚举（OWASP Authentication Cheat Sheet）：第三方注册（无密码）账号的登录失败，
// 响应必须与「账号不存在 / 密码错误」**完全一致**。
//
// 锁一个真实缺陷：此前的实现对 `ErrUserNoPassword` 单独返回
// 「该账号通过 {{Provider}} 注册，尚未设置密码…」，于是攻击者用
// 「用户名 + 任意密码」即可判定「该账号存在，且用哪家 provider 注册」——
// 这正是账号枚举的判定因子。
//
// 修法不是牺牲可用性：通用文案 `user.username_or_password_error` 本身已经包含
// 「若通过第三方登录注册、可能尚未设置密码，请改用对应方式登录」的引导，
// 所以用户得到的帮助没有减少，只是不再顺带确认账号是否存在。
// 精确原因（含 provider）保留在**审计日志**里，供运营排查。
func TestLoginDoesNotRevealAccountExistence(t *testing.T) {
	db := useIsolatedAuditDB(t)

	// 第三方注册的账号：有记录、无密码。
	oauthUser := model.User{
		Username: "oauth_only_" + common.GetRandomString(6),
		Source:   "discord", // 刻意选一个**不**在通用文案示例(NoneLoc/GitHub)里的 provider，这样任何出现即代表泄露
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&oauthUser).Error)
	require.Empty(t, oauthUser.Password, "前置条件：该账号必须没有密码")

	gin.SetMode(gin.TestMode)
	attempt := func(username string) (int, string) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := `{"username":"` + username + `","password":"any-wrong-password"}`
		c.Request = httptest.NewRequest("POST", "/api/user/login", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("login_method", "password")
		Login(c)
		var payload struct {
			Message string `json:"message"`
		}
		_ = common.Unmarshal(rec.Body.Bytes(), &payload)
		return rec.Code, payload.Message
	}

	codeExisted, msgExisted := attempt(oauthUser.Username)
	codeGhost, msgGhost := attempt("ghost_" + common.GetRandomString(8))

	assert.Equal(t, codeGhost, codeExisted,
		"状态码必须一致 —— 否则同样可用于枚举账号是否存在")
	assert.Equal(t, msgGhost, msgExisted,
		"响应文案必须一致 —— 否则可据此判定账号是否存在（账号枚举）")
	// 通用文案里本来就列了示例 provider（如 GitHub），所以只有**本例独有的** provider
	// 出现在响应里才算泄露。这里用 discord。
	assert.NotContains(t, strings.ToLower(msgExisted), "discord",
		"响应不得透露该账号的注册来源")
	assert.NotEmpty(t, msgExisted)

	// 精确原因仍必须进审计（内部可排查），且带上 provider。
	// 直接查原始 other 列：审计结构经 JSON 往返后 Op 可能为 nil，
	// 按结构断言会得到一个「假通过」。
	var otherTexts []string
	require.NoError(t, db.Raw("SELECT other FROM audit_logs").Scan(&otherTexts).Error)
	joined := strings.ToLower(strings.Join(otherTexts, " "))
	assert.Contains(t, joined, "no_password_use_oauth",
		"无密码账号的登录失败必须以 no_password_use_oauth 记入审计")
	assert.Contains(t, joined, "discord",
		"审计里应保留 provider，供运营判断是哪家 provider 的回调有问题")
}
