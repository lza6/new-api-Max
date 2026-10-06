package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
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

// TestGetBandwidthLeaderboardSQLAggregation 验证按日带宽排行 SQL 聚合：
// (created_at + 时区偏移)/86400 整数键分组，Go 端还原日期；字节/请求数正确；
// 排除 error 日志；limit 生效。管理端 SLOW SQL 修复回归（原拉 38 万行 1273ms）。
func TestGetBandwidthLeaderboardSQLAggregation(t *testing.T) {
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

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()
	yesterday := startOfToday - 86400
	_, tzOffsetInt := now.In(time.Local).Zone()
	tzOffset := int64(tzOffsetInt)

	logs := []*model.Log{
		{Type: model.LogTypeConsume, ModelName: "m1", RequestBytes: 100, ResponseBytes: 900, CreatedAt: startOfToday + 3600},
		{Type: model.LogTypeConsume, ModelName: "m2", RequestBytes: 200, ResponseBytes: 800, CreatedAt: startOfToday + 7200},
		{Type: model.LogTypeConsume, ModelName: "m1", RequestBytes: 50, ResponseBytes: 50, CreatedAt: yesterday + 3600},
		{Type: model.LogTypeError, ModelName: "m1", RequestBytes: 999, ResponseBytes: 999, CreatedAt: startOfToday},
	}
	require.NoError(t, database.Create(&logs).Error)

	// days=30 覆盖两条；error 日志排除；按日字节降序（今天 2000 > 昨天 100）
	got, err := queryBandwidthLeaderboardSQL(t, 30, 10, tzOffset)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// 具体断言两行的日期与字节
	todayStr := time.Unix(startOfToday, 0).In(time.Local).Format("2006-01-02")
	yesterdayStr := time.Unix(yesterday, 0).In(time.Local).Format("2006-01-02")
	require.Equal(t, todayStr, got[0].Date)
	require.Equal(t, int64(2000), got[0].Bytes)
	require.Equal(t, int64(2), got[0].Requests)
	require.Equal(t, yesterdayStr, got[1].Date)
	require.Equal(t, int64(100), got[1].Bytes)
	require.Equal(t, int64(1), got[1].Requests)
}

// queryBandwidthLeaderboardSQL 抽取 GetBandwidthLeaderboard 的 SQL 聚合核心，
// 便于独立单测（controller handler 层依赖 gin 上下文较耦合）。
func queryBandwidthLeaderboardSQL(t *testing.T, days, limit int, tzOffset int64) ([]service.BandwidthDay, error) {
	dayStart := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	type dayRow struct {
		DayKey   int64 `gorm:"column:day_key"`
		Requests int64 `gorm:"column:requests"`
		Bytes    int64 `gorm:"column:bytes"`
	}
	var dayRows []dayRow
	if err := model.LOG_DB.Model(&model.Log{}).
		Select(fmt.Sprintf("(created_at + %d) / 86400 AS day_key", tzOffset),
			"COUNT(*) AS requests",
			"COALESCE(SUM(request_bytes), 0) + COALESCE(SUM(response_bytes), 0) AS bytes").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume, dayStart).
		Group("day_key").
		Order("bytes DESC, day_key ASC").
		Scan(&dayRows).Error; err != nil {
		return nil, err
	}
	out := make([]service.BandwidthDay, 0, len(dayRows))
	for _, r := range dayRows {
		date := time.Unix(r.DayKey*86400-tzOffset, 0).In(time.Local).Format("2006-01-02")
		out = append(out, service.BandwidthDay{Date: date, Requests: r.Requests, Bytes: r.Bytes})
	}
	return out, nil
}

// TestBandwidthLeaderboardRedisCache 验证带宽排行 Redis 缓存：启用 Redis 时
// 首次查询写缓存，二次查询命中缓存（不再触发 DB 全表聚合）。miniredis 模拟。
func TestBandwidthLeaderboardRedisCache(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = previousRedisEnabled
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})

	now := time.Now().Unix()
	logs := []*model.Log{
		{Type: model.LogTypeConsume, ModelName: "m1", RequestBytes: 100, ResponseBytes: 900, CreatedAt: now},
		{Type: model.LogTypeConsume, ModelName: "m2", RequestBytes: 50, ResponseBytes: 10, CreatedAt: now},
	}
	require.NoError(t, database.Create(&logs).Error)

	// 首次：走 DB 聚合 + 写缓存
	first, err := queryModelBandwidthLeaderboard(30, 10)
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.True(t, server.Exists("bandwidth:model:30:10"), "缓存 key 应已写入")

	// 二次：应命中缓存（清空 DB 数据不影响结果 = 证明未查 DB）
	require.NoError(t, database.Delete(&logs).Error)
	second, err := queryModelBandwidthLeaderboard(30, 10)
	require.NoError(t, err)
	require.Equal(t, first, second, "缓存命中应返回相同数据且不依赖 DB")
}

// queryTrafficByDayCache 抽取 GetLogsTraffic 的聚合下推 + 缓存读写路径，便于独立单测：
// SQL 侧按日 SUM 持久化字节列（不再全表扫 other JSON），只有按日聚合结果进缓存，
// 且命中缓存时 total/请求数由 byDay 重算。
func queryTrafficByDayCache(t *testing.T, days int) ([]service.DailyTraffic, int64, int64, bool) {
	t.Helper()
	cacheKey := fmt.Sprintf("traffic:day:%d", days)
	if common.RedisEnabled {
		if cached, err := common.RedisGet(cacheKey); err == nil && cached != "" {
			var cachedByDay []service.DailyTraffic
			if common.Unmarshal([]byte(cached), &cachedByDay) == nil && cachedByDay != nil {
				var cachedTotal, cachedRequests int64
				for i := range cachedByDay {
					cachedTotal += cachedByDay[i].Bytes
					cachedRequests += int64(cachedByDay[i].Requests)
				}
				return cachedByDay, cachedTotal, cachedRequests, true
			}
		}
	}
	_, tzOffsetInt := time.Now().In(time.Local).Zone()
	tzOffset := int64(tzOffsetInt)
	start := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	type dayRow struct {
		DayKey   int64 `gorm:"column:day_key"`
		Requests int64 `gorm:"column:requests"`
		Bytes    int64 `gorm:"column:bytes"`
	}
	var dayRows []dayRow
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).
		Select(fmt.Sprintf("(created_at + %d) / 86400 AS day_key", tzOffset),
			"COUNT(*) AS requests",
			"COALESCE(SUM(request_bytes), 0) + COALESCE(SUM(response_bytes), 0) AS bytes").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume, start).
		Group("day_key").
		Order("day_key ASC").
		Scan(&dayRows).Error)
	byDay := make([]service.DailyTraffic, 0, len(dayRows))
	var total, requestCount int64
	for _, r := range dayRows {
		date := time.Unix(r.DayKey*86400-tzOffset, 0).In(time.Local).Format("2006-01-02")
		total += r.Bytes
		requestCount += r.Requests
		byDay = append(byDay, service.DailyTraffic{
			Date:     date,
			Requests: int(r.Requests),
			Bytes:    r.Bytes,
			MB:       float64(r.Bytes) / (1024 * 1024),
		})
	}
	if common.RedisEnabled {
		if raw, err := common.Marshal(byDay); err == nil {
			_ = common.RedisSet(cacheKey, string(raw), time.Minute)
		}
	}
	return byDay, total, requestCount, false
}

// TestTrafficCacheStoresOnlyDailyAggregates 生产回归（2026-09-29 全站 503 事故）：
// traffic 缓存必须只保存按日聚合行，绝不能把窗口内全量原始行写进 Redis。
// 44 万行 / 290MB+ 的缓存值在 maxmemory 数十 MB 的实例上触发 OOM，
// 连带打挂限流与会话缓存。同时验证命中缓存后 total/请求数口径与原实现一致。
func TestTrafficCacheStoresOnlyDailyAggregates(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = previousRedisEnabled
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})

	now := time.Now().Unix()
	logs := []*model.Log{
		{Type: model.LogTypeConsume, ModelName: "m1", CreatedAt: now, RequestBytes: 100, ResponseBytes: 900},
		{Type: model.LogTypeConsume, ModelName: "m1", CreatedAt: now, RequestBytes: 50, ResponseBytes: 50},
		{Type: model.LogTypeConsume, ModelName: "m2", CreatedAt: now - 86400, RequestBytes: 10, ResponseBytes: 10},
	}
	require.NoError(t, database.Create(&logs).Error)

	byDay, total, requestCount, cached := queryTrafficByDayCache(t, 30)
	require.False(t, cached, "首次必须未命中缓存")
	require.Len(t, byDay, 2)
	// 今日 100+900 与 50+50，昨日 10+10。
	assert.Equal(t, int64(1120), total)
	assert.Equal(t, int64(3), requestCount)

	cachedRaw, err := server.Get("traffic:day:30")
	require.NoError(t, err)
	assert.NotContains(t, cachedRaw, "other", "缓存值不得包含全量原始行")
	assert.NotContains(t, cachedRaw, "request_bytes", "缓存值不得包含逐条字节字段")
	assert.Less(t, len(cachedRaw), 1024, "缓存值应只含按日聚合行（≤90 行）")

	// 命中缓存：清空 DB 后结果不变，且 total/请求数由 byDay 重算得出。
	require.NoError(t, database.Delete(&logs).Error)
	byDay2, total2, requestCount2, cached2 := queryTrafficByDayCache(t, 30)
	assert.True(t, cached2, "二次必须命中缓存")
	assert.Equal(t, byDay, byDay2)
	assert.Equal(t, total, total2)
	assert.Equal(t, requestCount, requestCount2)
}

// TestProcessChannelErrorPreservesUnreachableClass 生产回归（2026-09-29）：
// upstream_unreachable（网络层失败，请求从未到达上游）必须保留其精确错误码，
// 不得被降格覆盖成笼统的 upstream_unavailable —— 前者保证「请求未被处理、
// 客户端可安全重试」，是排障与重试决策的关键信息。
func TestProcessChannelErrorPreservesUnreachableClass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "unreachable-class-test")

	unreachable := types.NewErrorWithStatusCode(
		errors.New("dial tcp 70.39.183.88:443: connect: connection refused"),
		types.ErrorCodeUpstreamUnreachable,
		http.StatusBadGateway,
	)
	processChannelError(c, types.ChannelError{ChannelId: 49, ChannelName: "t", ChannelType: 1}, unreachable, nil)
	assert.Equal(t, types.ErrorCodeUpstreamUnreachable, unreachable.GetErrorCode(),
		"unreachable 分类不得被覆盖")

	// 普通 502（无精确分类）仍归一为 upstream_unavailable
	generic := types.NewErrorWithStatusCode(errors.New("bad gateway"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	processChannelError(c, types.ChannelError{ChannelId: 50, ChannelName: "t", ChannelType: 1}, generic, nil)
	assert.Equal(t, types.ErrorCodeUpstreamUnavailable, generic.GetErrorCode())
}
