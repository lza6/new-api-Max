package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	// P0-4 错误归因：502 → server_error 稳定短标识落日志，前端可直接人话映射。
	assert.Equal(t, "server_error", storedOther["error_class"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogTypeError, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}

// TestProcessChannelErrorClassifyToB62 B6-2：processChannelError 按 B2-1
// 错误类把通用错误归一为机器可读 error.type。
func TestProcessChannelErrorClassifyToB62(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = false // 不落错误日志，仅断言错误码改写
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	channelSnapshot := types.ChannelError{ChannelId: 1, ChannelType: 1, AutoBan: false}

	run := func(statusCode int, code types.ErrorCode) *types.NewAPIError {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Set("id", 1)
		ctx.Set("username", "u")
		ctx.Set("token_name", "tk")
		ctx.Set("token_id", 1)
		ctx.Set("original_model", "m")
		ctx.Set("group", "default")
		ctx.Set("channel_id", 1)
		common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now())
		apiErr := types.NewOpenAIError(errors.New("upstream"), code, statusCode)
		processChannelError(ctx, channelSnapshot, apiErr, nil)
		return apiErr
	}

	// 401 → key_invalid；429 → rate_limited；5xx → upstream_unavailable；
	// prompt_blocked → content_filtered；业务渠道码（channel:invalid_key）保留。
	assert.Equal(t, types.ErrorCodeKeyInvalid, run(http.StatusUnauthorized, types.ErrorCodeBadResponseStatusCode).GetErrorCode())
	assert.Equal(t, types.ErrorCodeRateLimited, run(http.StatusTooManyRequests, types.ErrorCodeBadResponseStatusCode).GetErrorCode())
	assert.Equal(t, types.ErrorCodeUpstreamUnavailable, run(http.StatusBadGateway, types.ErrorCodeBadResponseStatusCode).GetErrorCode())
	assert.Equal(t, types.ErrorCodeContentFiltered, run(http.StatusBadRequest, types.ErrorCodePromptBlocked).GetErrorCode())
	// 403：超额语义 → insufficient_quota（不被误判为坏 key）。
	assert.Equal(t, types.ErrorCodeInsufficientQuota, run(http.StatusForbidden, types.ErrorCodeBadResponseStatusCode).GetErrorCode())
	// 上游 content_filter 变体 → content_filtered。
	assert.Equal(t, types.ErrorCodeContentFiltered, run(http.StatusBadRequest, types.ErrorCodeContentFiltered).GetErrorCode())
	assert.Equal(t, types.ErrorCodeContentFiltered, run(http.StatusBadRequest, types.ErrorCodeSensitiveWordsDetected).GetErrorCode())
	channelErr := types.NewOpenAIError(errors.New("key"), types.ErrorCodeChannelInvalidKey, http.StatusUnauthorized)
	processChannelErrorWithError(t, channelSnapshot, channelErr)
	assert.Equal(t, types.ErrorCodeChannelInvalidKey, channelErr.GetErrorCode())
}

// processChannelErrorWithError 复用同一快照直接调用，避免 table 里重复上下文。
func processChannelErrorWithError(t *testing.T, ch types.ChannelError, apiErr *types.NewAPIError) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 1)
	ctx.Set("username", "u")
	ctx.Set("token_name", "tk")
	ctx.Set("token_id", 1)
	ctx.Set("original_model", "m")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 1)
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now())
	processChannelError(ctx, ch, apiErr, nil)
}

// TestQueryModelBandwidthLeaderboardSQLAggregation 验证 SQL GROUP BY 聚合路径
// 与旧的 Go 内存聚合语义等价（按 model 聚合字节、空模型名 -> "(unknown)"、
// limit 生效、降序）。慢查询修复回归：原实现拉全量行进内存（线上 37 万行
// 541ms），现改为数据库聚合只返回 Top-N。
func TestQueryModelBandwidthLeaderboardSQLAggregation(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})

	now := time.Now().Unix()
	logs := []*model.Log{
		{Type: model.LogTypeConsume, ModelName: "deepseek-v4-flash", RequestBytes: 100, ResponseBytes: 900, CreatedAt: now},
		{Type: model.LogTypeConsume, ModelName: "deepseek-v4-flash", RequestBytes: 200, ResponseBytes: 800, CreatedAt: now},
		{Type: model.LogTypeConsume, ModelName: "gpt-5.6-sol", RequestBytes: 50, ResponseBytes: 10, CreatedAt: now},
		{Type: model.LogTypeConsume, ModelName: "", RequestBytes: 10, ResponseBytes: 20, CreatedAt: now},
		{Type: model.LogTypeError, ModelName: "deepseek-v4-flash", RequestBytes: 5, ResponseBytes: 5, CreatedAt: now},
	}
	require.NoError(t, database.Create(&logs).Error)

	got, err := queryModelBandwidthLeaderboard(30, 10)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, "deepseek-v4-flash", got[0].Model)
	require.Equal(t, int64(2), got[0].Requests)
	require.Equal(t, int64(2000), got[0].Bytes)
	require.Equal(t, "gpt-5.6-sol", got[1].Model)
	require.Equal(t, int64(1), got[1].Requests)
	require.Equal(t, int64(60), got[1].Bytes)
	require.Equal(t, "(unknown)", got[2].Model)
	require.Equal(t, int64(30), got[2].Bytes)

	// limit 生效：只返回字节数最大的 1 个模型
	limited, err := queryModelBandwidthLeaderboard(30, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	require.Equal(t, "deepseek-v4-flash", limited[0].Model)
	require.Equal(t, int64(2000), limited[0].Bytes)

	// 窗口过滤：插入一条超窗（2 天前）数据，days=1 应排除它
	old := &model.Log{Type: model.LogTypeConsume, ModelName: "old-model", RequestBytes: 999, ResponseBytes: 999, CreatedAt: now - 2*86400}
	require.NoError(t, database.Create(old).Error)
	filtered, err := queryModelBandwidthLeaderboard(1, 10)
	require.NoError(t, err)
	for _, m := range filtered {
		require.NotEqual(t, "old-model", m.Model, "超窗数据不应出现在 days=1 结果中")
	}
}
