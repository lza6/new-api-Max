package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
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

// --- 账号级登录失败锁定（OWASP Authentication Cheat Sheet — Account Lockout） ---
//
// 这两条用例锁住"锁定真的生效"与"锁定不给受害者造成持久伤害"两件事：
//  1. 账号级计数达阈值后，**即使密码正确也被拒**，且响应与"密码错误/账号不存在"
//     逐字节一致（状态码 + 文案）——锁定不能被用作账号枚举的新判定因子。
//  2. 一次成功认证立刻清零计数：攻击者的锁定无法在受害者登录后继续存在。
//
// 夹具：沿用 TestMain 的共享全量库（含 users/auth_flows/audit_logs），
// 每个用例用唯一用户名，避免共享库上的跨用例污染。

// withLoginLockoutPolicy 覆盖账号级锁定参数，用例结束后恢复（并放行已锁账号）。
func withLoginLockoutPolicy(t *testing.T, apply func(*operation_setting.LoginLockoutSetting)) {
	t.Helper()
	previous := *operation_setting.GetLoginLockoutSetting()
	previousPasswordLogin := common.PasswordLoginEnabled
	previousEncryption := common.PasswordLoginEncryptionEnabled
	t.Cleanup(func() {
		*operation_setting.GetLoginLockoutSetting() = previous
		common.PasswordLoginEnabled = previousPasswordLogin
		common.PasswordLoginEncryptionEnabled = previousEncryption
	})
	apply(operation_setting.GetLoginLockoutSetting())
	common.PasswordLoginEnabled = true
	common.PasswordLoginEncryptionEnabled = false
}

// useIsolatedLoginDB 让本用例自持数据库。
//
// **为什么必须自持**：TestMain 的共享 in-memory 库会被同包其它用例关闭/改写
// （全量执行时 `TestAuditDatabaseMatrix` 之后 model.DB 即处于关闭状态，
// 后续用例统一报 `sql: database is closed`）。依赖共享库会让本用例变成
// 「只在单独跑时通过」的顺序依赖用例——那等于没有回归保护。
// 这与 v1.3.138「访客端点用例自持数据库」是同一条纪律。
func useIsolatedLoginDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.AuthFlow{},
		&model.TwoFA{}, &model.PasskeyCredential{}, &model.AuditLog{},
	))
	model.DB, model.LOG_DB = db, db
	// 关 Redis：失败台账是本进程内存实现，且登录路径不应依赖外部缓存。
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		_ = sqlDB.Close()
	})
	return db
}

// createPasswordLoginUser 建一个可密码登录的用户（唯一用户名）。
func createPasswordLoginUser(t *testing.T, prefix, password string) *model.User {
	t.Helper()
	hashed, err := common.Password2Hash(password)
	require.NoError(t, err)
	user := &model.User{
		Username:    prefix + "_" + common.GetRandomString(8),
		Password:    hashed,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		// aff_code 是唯一索引；model.CreateUser 会在正常注册路径填充它，
		// 这里直接 DB.Create 必须自己给一个唯一值。
		AffCode: common.GetRandomString(6),
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

// loginAttempt 一次真实 HTTP 密码登录的结果。
// 注意：本项目业务错误用 HTTP 200 + `success:false` 表达（common.ApiErrorI18n），
// 因此"是否成功"必须读 success 字段，而不是 HTTP 状态码。
type loginAttempt struct {
	httpStatus int
	success    bool
	message    string
}

func attemptPasswordLogin(t *testing.T, username, password string) loginAttempt {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := `{"username":` + strconv.Quote(username) + `,"password":` + strconv.Quote(password) + `}`
	c.Request = httptest.NewRequest("POST", "/api/user/login", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("login_method", "password")
	Login(c)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	_ = common.Unmarshal(rec.Body.Bytes(), &payload)
	return loginAttempt{httpStatus: rec.Code, success: payload.Success, message: payload.Message}
}

func TestLoginAccountLockoutBlocksPasswordLoginWithoutRevealingAccountState(t *testing.T) {
	useIsolatedLoginDB(t)
	withLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 300
		s.MaxLockSeconds = 900
	})

	user := createPasswordLoginUser(t, "lockout_block", "correct-password-value")

	// 对照组：不存在账号的错误密码响应，作为"通用文案"的基准。
	ghost := attemptPasswordLogin(t, "ghost_"+common.GetRandomString(8), "wrong-password")
	require.False(t, ghost.success)

	// 打满阈值。
	for attempt := 1; attempt <= 2; attempt++ {
		failed := attemptPasswordLogin(t, user.Username, "wrong-password")
		assert.False(t, failed.success, "第 %d 次错误密码必须失败", attempt)
		if attempt == 1 {
			// 单次失败不得触发阈值 2 的锁定：防止"同一次失败被重复记账"
			// 把生效阈值悄悄砍半（配置的阈值必须就是真实生效的阈值）。
			assert.False(t, service.AccountLockoutStatus(user.Username).Locked,
				"1 次失败（阈值 2）不得锁定——否则说明同一次失败被重复计数")
		}
	}

	// 关键断言：被锁定后**即便密码正确**也拒绝，且响应与"密码错误/账号不存在"一致。
	locked := attemptPasswordLogin(t, user.Username, "correct-password-value")
	assert.False(t, locked.success, "锁定期间密码正确也必须拒绝")
	assert.Equal(t, ghost.message, locked.message,
		"锁定响应必须与通用失败文案逐字一致，否则锁定状态本身成为账号枚举判定因子")
	assert.Equal(t, ghost.httpStatus, locked.httpStatus, "HTTP 状态码同样不得有差异")

	// 锁定不得签发会话。
	var sessions int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Where("user_id = ?", user.Id).Count(&sessions).Error)
	assert.Zero(t, sessions, "被锁定账号不得签发任何登录会话")

	// 精确原因必须留在审计里（供运营回答"用户说登不上"的工单）。
	var otherTexts []string
	require.NoError(t, model.LOG_DB.Raw("SELECT other FROM audit_logs WHERE username = ?", user.Username).
		Scan(&otherTexts).Error)
	require.NotEmpty(t, otherTexts, "锁定尝试必须留审计")
	joined := strings.ToLower(strings.Join(otherTexts, " "))
	assert.Contains(t, joined, "account_locked", "审计必须记录 account_locked 原因")
	assert.NotContains(t, joined, "correct-password-value", "审计绝不得包含密码")
}

func TestLoginAccountLockoutResetsOnSuccessfulPasswordLogin(t *testing.T) {
	useIsolatedLoginDB(t)
	withLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 300
		s.MaxLockSeconds = 900
	})

	user := createPasswordLoginUser(t, "lockout_reset", "correct-password-value")

	// 1 次失败（未达阈值 2）+ 1 次成功 → 计数必须清零。
	require.False(t, attemptPasswordLogin(t, user.Username, "wrong-password").success)

	succeeded := attemptPasswordLogin(t, user.Username, "correct-password-value")
	require.True(t, succeeded.success, "正确密码必须登录成功（message=%s）", succeeded.message)

	// 成功后计数已清零：再来一次失败属于"第 1 次"，不得触发阈值 2 的锁定。
	require.False(t, attemptPasswordLogin(t, user.Username, "wrong-password").success)
	assert.False(t, service.AccountLockoutStatus(user.Username).Locked,
		"成功登录必须清零失败计数——否则受害者正常登录后仍只差一次失败就被锁")
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
