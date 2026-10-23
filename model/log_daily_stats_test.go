package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLogDailyStatsDB 建内存 SQLite 日志库，专供日汇总表测试。
func setupLogDailyStatsDB(t *testing.T) *gorm.DB {
	t.Helper()
	origDB, origLogDB := DB, LOG_DB
	origMain, origLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &LogDailyStat{}))
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = origDB, origLogDB
		common.SetDatabaseTypes(origMain, origLog)
	})
	return db
}

// 汇总表的数值必须与「直接对 logs 聚合」完全一致——它是查询侧的唯一真相源，
// 一旦口径偏移，统计页与财务面板会同时说谎。
func TestRefreshLogDailyStatsMatchesRawAggregate(t *testing.T) {
	db := setupLogDailyStatsDB(t)

	now := time.Now().Unix()
	yesterday := now - 86400
	rows := []Log{
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "m-a", PromptTokens: 10, CompletionTokens: 5, Quota: 100, RequestBytes: 1000, ResponseBytes: 500},
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "m-b", PromptTokens: 20, CompletionTokens: 7, Quota: 200, RequestBytes: 2000, ResponseBytes: 600},
		{Type: LogTypeConsume, CreatedAt: yesterday, ModelName: "m-a", PromptTokens: 30, CompletionTokens: 9, Quota: 300, RequestBytes: 3000, ResponseBytes: 700},
		// 非消费日志（错误/充值）绝不能计入消费统计
		{Type: LogTypeError, CreatedAt: now, ModelName: "m-a", PromptTokens: 999, Quota: 999},
	}
	require.NoError(t, db.Create(&rows).Error)

	require.NoError(t, RefreshLogDailyStats(7))

	var raw LogDailyAggRow
	require.NoError(t, db.Table("logs").
		Select("COUNT(*) AS requests, "+
			"COALESCE(SUM(request_bytes),0)+COALESCE(SUM(response_bytes),0) AS bytes, "+
			"COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, "+
			"COALESCE(SUM(completion_tokens),0) AS completion_tokens, "+
			"COALESCE(SUM(quota),0) AS quota").
		Where("type = ?", LogTypeConsume).
		Scan(&raw).Error)

	totals, err := GetSiteTotalsFromDailyStats(0)
	require.NoError(t, err)
	assert.Equal(t, raw.Requests, totals.Requests, "requests 必须与原始聚合一致")
	assert.Equal(t, raw.Bytes, totals.Bytes, "bytes 必须与原始聚合一致")
	assert.Equal(t, raw.PromptTokens, totals.PromptTokens, "prompt tokens 必须一致")
	assert.Equal(t, raw.CompletionTokens, totals.CompletionTokens, "completion tokens 必须一致")
	assert.Equal(t, raw.Quota, totals.Quota, "quota 必须一致")
	assert.EqualValues(t, 3, totals.Requests, "type=5 的错误日志不得计入")

	// model 维度：跨天按模型汇总
	models, err := GetModelStatsByDay(0)
	require.NoError(t, err)
	byModel := make(map[string]LogDailyAggRow, len(models))
	for _, m := range models {
		v := byModel[m.DimValue]
		v.Requests += m.Requests
		v.Bytes += m.Bytes
		byModel[m.DimValue] = v
	}
	assert.EqualValues(t, 2, byModel["m-a"].Requests)
	assert.EqualValues(t, 5200, byModel["m-a"].Bytes, "1500 + 3700")
	assert.EqualValues(t, 1, byModel["m-b"].Requests)
	assert.EqualValues(t, 2600, byModel["m-b"].Bytes)
}

// 刷新是「删窗口 + 重插」，重复执行不得产生重复行或重复累加。
func TestRefreshLogDailyStatsIsIdempotent(t *testing.T) {
	db := setupLogDailyStatsDB(t)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&Log{Type: LogTypeConsume, CreatedAt: now, ModelName: "m", Quota: 10}).Error)

	require.NoError(t, RefreshLogDailyStats(7))
	var first int64
	require.NoError(t, db.Model(&LogDailyStat{}).Count(&first).Error)

	require.NoError(t, RefreshLogDailyStats(7))
	var second int64
	require.NoError(t, db.Model(&LogDailyStat{}).Count(&second).Error)
	assert.Equal(t, first, second, "重复刷新不得产生重复行")

	totals, err := GetSiteTotalsFromDailyStats(0)
	require.NoError(t, err)
	assert.EqualValues(t, 10, totals.Quota, "重复刷新不得重复累加")
}

// 覆盖判断是查询侧切换数据源的前提：只判断「非空」会让未回填的历史静默漏算。
func TestDailyStatsCoverageAndEmpty(t *testing.T) {
	db := setupLogDailyStatsDB(t)
	assert.True(t, DailyStatsEmpty(), "空表必须报告 empty")
	assert.False(t, DailyStatsCovers(DayKeyFor(time.Now().Unix())), "空表不得声称覆盖")

	now := time.Now().Unix()
	// 汇总表只保存「有数据的天」，因此至少要有两天的数据才能验证跨天覆盖。
	require.NoError(t, db.Create(&[]Log{
		{Type: LogTypeConsume, CreatedAt: now, ModelName: "m", Quota: 1},
		{Type: LogTypeConsume, CreatedAt: now - 86400, ModelName: "m", Quota: 2},
	}).Error)
	require.NoError(t, RefreshLogDailyStats(7))

	assert.False(t, DailyStatsEmpty())
	assert.True(t, DailyStatsCovers(DayKeyFor(now-86400)), "应覆盖今天与前一天")
	assert.False(t, DailyStatsCovers(DayKeyFor(now-40*86400)), "未回填 40 天前，不得声称覆盖")
}

// day_key 必须与 SQL 侧 LogDayKeyExpr 同口径：86400 的整数倍 + 对应本地零点。
func TestDayKeyForMatchesSQLLogic(t *testing.T) {
	ts := time.Now().Unix()
	dk := DayKeyFor(ts)
	assert.Zero(t, dk%86400, "day_key 必须是 86400 的整数倍")
	assert.Equal(t, time.Now().In(time.Local).Format("2006-01-02"), DayKeyToDate(dk))
	// 同一本地自然日内的任意时刻映射到同一个 day_key
	assert.Equal(t, dk, DayKeyFor(dk+1))
}
