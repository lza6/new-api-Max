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
