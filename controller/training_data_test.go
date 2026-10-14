package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service/training_capture"
)

// 采集接口的**安全契约**测试。
//
// 这组用例的重点不是「接口能返回 200」，而是几条越权/绕过路径被堵住：
//  1. 用户 ID 只从会话取 —— 请求体里塞别人的 ID 无效；
//  2. 不能用一个「不存在的旧声明版本」同意，从而绕过版本校验；
//  3. 云盘未配置时不能打开总开关（否则数据无处可去却显示「已开启」）；
//  4. 撤回后立即不可采集（无 30s 缓存窗口）。
//
// 这些是合规与隐私边界，比功能本身更重要。

// setupTrainingDataTestDB 建一个只含采集两张表的 SQLite 库。
func setupTrainingDataTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ctrl-test.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "打开测试 SQLite")

	require.NoError(t, db.AutoMigrate(
		&model.TrainingDataConsent{}, &model.TrainingDataWatermark{}))

	previous := model.DB
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previous
	})
}

// jsonContext 构造一个带会话用户 ID 的 gin 测试上下文。
//
// `sessionUserId` 模拟中间件从会话里解出的用户；它**不应**被请求体覆盖。
func jsonContext(t *testing.T, method, target, body string, sessionUserId int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if sessionUserId != 0 {
		ctx.Set("id", sessionUserId)
	}
	return ctx, recorder
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ---------- 用户端 ----------

func TestGrantTrainingDataConsentUsesSessionUserIdNotBody(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(fakeSinkForController{})()
	defer training_capture.SetGatewayEnabledForTest(true)()
	training_capture.ResetConsentCacheForTest()

	// 会话身份是 42，请求体里塞一个**别人的** user_id=999。
	ctx, recorder := jsonContext(t, http.MethodPost, "/api/user/training-data/grant",
		`{"user_id":999,"policy_version":"`+training_capture.CurrentPolicyVersion+`"}`, 42)

	GrantTrainingDataConsent(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success, "请求应成功（对会话用户 42）")

	// **关键断言**：被授权的是 42，不是 999。
	granted, err := model.GetTrainingDataConsent(42)
	require.NoError(t, err)
	require.NotNil(t, granted, "会话用户 42 应被授权")
	assert.True(t, granted.Granted)

	other, err := model.GetTrainingDataConsent(999)
	require.NoError(t, err)
	assert.Nil(t, other, "请求体里的 999 绝不能被授权（越权写）")
}

func TestGrantRejectsMismatchedPolicyVersion(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(fakeSinkForController{})()
	defer training_capture.SetGatewayEnabledForTest(true)()
	training_capture.ResetConsentCacheForTest()

	// 用户「同意」一个服务端不存在的旧版本 —— 必须拒绝，
	// 否则可以绕过「声明更新后需重新同意」的合规要求。
	ctx, recorder := jsonContext(t, http.MethodPost, "/api/user/training-data/grant",
		`{"policy_version":"v0-old"}`, 7)

	GrantTrainingDataConsent(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success, "版本不匹配必须拒绝")
	assert.Contains(t, env.Message, "policy version mismatch")

	consent, err := model.GetTrainingDataConsent(7)
	require.NoError(t, err)
	assert.Nil(t, consent, "被拒绝的请求不得写入任何授权记录")
}

func TestGrantAcceptsServerVersionAndEmptyBody(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(fakeSinkForController{})()
	defer training_capture.SetGatewayEnabledForTest(true)()
	training_capture.ResetConsentCacheForTest()

	// 空请求体（不传版本）应被接受并使用服务端当前版本。
	ctx, recorder := jsonContext(t, http.MethodPost, "/api/user/training-data/grant", ``, 8)
	GrantTrainingDataConsent(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success, "空请求体应被接受: %s", env.Message)

	consent, err := model.GetTrainingDataConsent(8)
	require.NoError(t, err)
	require.NotNil(t, consent)
	assert.Equal(t, training_capture.CurrentPolicyVersion, consent.PolicyVersion,
		"必须记录**服务端**当前版本，而不是空值")
}

func TestRevokeStopsCollectionImmediately(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(fakeSinkForController{})()
	defer training_capture.SetGatewayEnabledForTest(true)()
	training_capture.ResetConsentCacheForTest()

	const userId = 55
	// 先授权并确认可采集。
	grantCtx, _ := jsonContext(t, http.MethodPost, "/api/user/training-data/grant", ``, userId)
	GrantTrainingDataConsent(grantCtx)
	require.True(t, training_capture.IsGranted(userId), "刚授权应可采集")

	// 撤回。
	revokeCtx, recorder := jsonContext(t, http.MethodPost, "/api/user/training-data/revoke", ``, userId)
	RevokeTrainingDataConsent(revokeCtx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success)

	// **关键**：撤回后**立即**不可采集，不能有缓存残留窗口。
	assert.False(t, training_capture.IsGranted(userId),
		"撤回后必须立即停止采集（缓存不得残留已授权状态）")

	consent, err := model.GetTrainingDataConsent(userId)
	require.NoError(t, err)
	require.NotNil(t, consent, "撤回应保留记录（合规留痕），不是删行")
	assert.False(t, consent.Granted)
	assert.NotZero(t, consent.RevokedAt, "必须记录撤回时间")
}

func TestStatusReportsUnavailableWhenSinkMissing(t *testing.T) {
	setupTrainingDataTestDB(t)
	// 云盘未配置。
	defer training_capture.SetSinkForTest(nil)()
	defer training_capture.SetGatewayEnabledForTest(true)()
	training_capture.ResetConsentCacheForTest()

	ctx, recorder := jsonContext(t, http.MethodGet, "/api/user/training-data/status", ``, 3)
	GetTrainingDataStatus(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success)

	var status training_capture.ConsentStatus
	require.NoError(t, common.Unmarshal(env.Data, &status))
	// 云盘没配 → 对用户不可用（前端据此不渲染开关）。
	assert.False(t, status.Available, "云盘未配置时必须报告不可用")
	assert.NotEmpty(t, status.Reason, "必须给出可读原因")
}

// ---------- 管理端 ----------

func TestSetTrainingDataEnabledRejectsEnablingWithoutSink(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(nil)()
	defer training_capture.SetGatewayEnabledForTest(false)()
	training_capture.ResetConsentCacheForTest()

	ctx, recorder := jsonContext(t, http.MethodPut, "/api/training-data/admin/enabled",
		`{"enabled":true}`, 1)

	SetTrainingDataEnabled(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	// 云盘没配就允许「打开」会让管理员以为在采集（数据全被丢）。
	assert.False(t, env.Success, "云盘未配置时不得允许开启采集")
	assert.Contains(t, env.Message, "not configured")
	assert.False(t, training_capture.IsGatewayEnabled(), "被拒绝后开关状态不应改变")
}

func TestSetTrainingDataEnabledTogglesAndPersists(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(fakeSinkForController{})()
	training_capture.SetGatewayEnabled(true)

	// 关闭。
	offCtx, offRec := jsonContext(t, http.MethodPut, "/api/training-data/admin/enabled",
		`{"enabled":false}`, 1)
	SetTrainingDataEnabled(offCtx)
	var offEnv apiEnvelope
	require.NoError(t, common.Unmarshal(offRec.Body.Bytes(), &offEnv))
	require.True(t, offEnv.Success, "关闭应成功: %s", offEnv.Message)
	assert.False(t, training_capture.IsGatewayEnabled())
	assert.False(t, training_capture.HotPathEnabled(),
		"关闭后热路径快速开关必须同步关闭（否则请求路径仍在做无用工作）")

	// 再打开（云盘已配）。
	onCtx, onRec := jsonContext(t, http.MethodPut, "/api/training-data/admin/enabled",
		`{"enabled":true}`, 1)
	SetTrainingDataEnabled(onCtx)
	var onEnv apiEnvelope
	require.NoError(t, common.Unmarshal(onRec.Body.Bytes(), &onEnv))
	require.True(t, onEnv.Success, "开启应成功: %s", onEnv.Message)
	assert.True(t, training_capture.IsGatewayEnabled())
	assert.True(t, training_capture.HotPathEnabled())
}

func TestSetTrainingDataEnabledRejectsMalformedBody(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(fakeSinkForController{})()

	cases := []struct {
		name string
		body string
	}{
		{"空对象（缺 enabled）", `{}`},
		{"enabled 类型错误", `{"enabled":"yes"}`},
		{"非法 JSON", `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := training_capture.IsGatewayEnabled()
			ctx, recorder := jsonContext(t, http.MethodPut,
				"/api/training-data/admin/enabled", tc.body, 1)
			SetTrainingDataEnabled(ctx)

			var env apiEnvelope
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
			assert.False(t, env.Success, "畸形请求必须被拒绝")
			// 关键：被拒绝的请求不得改变状态（不能"部分成功"）。
			assert.Equal(t, before, training_capture.IsGatewayEnabled(),
				"被拒绝的请求不得改动开关")
		})
	}
}

func TestByDateRejectsInvalidFormat(t *testing.T) {
	setupTrainingDataTestDB(t)

	ctx, recorder := jsonContext(t, http.MethodGet,
		"/api/training-data/admin/by-date?date=2026-13-99", ``, 1)
	GetTrainingDataByDate(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success, "非法日期必须被拒绝")
}

func TestByDateForPastDateReportsUnknownInsteadOfFaking(t *testing.T) {
	setupTrainingDataTestDB(t)

	// 请求一个历史日期：进程内只保留当日计数，必须**如实说明**，
	// 不能返回今天的数字冒充历史（那会让管理员误判）。
	ctx, recorder := jsonContext(t, http.MethodGet,
		"/api/training-data/admin/by-date?date=2020-01-01", ``, 1)
	GetTrainingDataByDate(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success)

	var payload struct {
		Known  bool   `json:"known"`
		Reason string `json:"reason"`
		Stats  any    `json:"stats"`
	}
	require.NoError(t, common.Unmarshal(env.Data, &payload))
	assert.False(t, payload.Known, "历史日期必须报告 unknown")
	assert.NotEmpty(t, payload.Reason, "必须说明为什么拿不到")
	assert.Nil(t, payload.Stats, "不得用当日数字冒充历史数据")
}

func TestOverviewReportsQuotaUnknownWhenSinkFails(t *testing.T) {
	setupTrainingDataTestDB(t)
	// 用一个 Stat 会报错的 sink：前端必须显示「未知」而不是 0。
	defer training_capture.SetSinkForTest(failingStatSink{})()
	defer training_capture.SetGatewayEnabledForTest(true)()

	ctx, recorder := jsonContext(t, http.MethodGet,
		"/api/training-data/admin/overview", ``, 1)
	GetTrainingDataAdminOverview(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success)

	var overview TrainingDataOverview
	require.NoError(t, common.Unmarshal(env.Data, &overview))
	assert.False(t, overview.Sink.QuotaKnown, "查询失败时不得声称配额已知")
	assert.NotEmpty(t, overview.Sink.LastError, "必须带上失败原因")
}

// ---------- 测试替身 ----------

// fakeSinkForController 满足 TrainingDataSink 的最小实现。
type fakeSinkForController struct{}

func (fakeSinkForController) Name() string { return "fake" }

func (fakeSinkForController) EnsureDir(ctx context.Context, path string) error { return nil }

func (fakeSinkForController) WritePart(ctx context.Context, path string, data []byte) (string, error) {
	return "ref", nil
}

func (fakeSinkForController) List(ctx context.Context, path string) ([]string, error) {
	return nil, nil
}

func (fakeSinkForController) Stat(ctx context.Context) (training_capture.SinkStat, error) {
	return training_capture.SinkStat{
		Provider: "fake", TotalBytes: 1 << 40, RemainingBytes: 1 << 40,
	}, nil
}

// failingStatSink 配额查询必失败（模拟云盘挂掉）。
type failingStatSink struct{}

func (failingStatSink) Name() string { return "failing" }

func (failingStatSink) EnsureDir(ctx context.Context, path string) error { return nil }

func (failingStatSink) WritePart(ctx context.Context, path string, data []byte) (string, error) {
	return "ref", nil
}

func (failingStatSink) List(ctx context.Context, path string) ([]string, error) {
	return nil, nil
}

func (failingStatSink) Stat(ctx context.Context) (training_capture.SinkStat, error) {
	return training_capture.SinkStat{}, assert.AnError
}
