package service

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// 公开状态页（G7）的契约测试。
//
// 这一层要锁死的是**口径**，不是渲染：
//  1. 流量数字受日志开关门控 —— 开关关掉时必须返回"不可用"（nil），
//     绝不能拿 0 冒充"没有请求"，更不能拿"没有错误记录"冒充 100% 成功率；
//  2. 统计窗口必须真实生效（窗口外的日志不计入），且窗口长度随响应返回；
//  3. 组件状态来自真实探测（依赖缺失 = down；未配置 = disabled，不是 down）；
//     依赖全空时**不得 panic**（公开端点）；
//  4. 后台任务状态只依据心跳时间戳，且判定窗口随响应返回。

// setupPublicStatusLogDB 挂内存 SQLite 作为主库/日志库，并清空状态页缓存。
func setupPublicStatusLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevDB, prevLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = db, db
	ResetPublicStatusCacheForTest()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLogDB
		ResetPublicStatusCacheForTest()
	})
	return db
}

// publicStatusForTest 执行一次聚合（绕过 SWR 缓存，便于在同一用例内切换开关后重算）。
func publicStatusForTest(t *testing.T) *PublicStatus {
	t.Helper()
	status, err := computePublicStatus()
	require.NoError(t, err)
	return status
}

// setErrorLogging 覆盖 ERROR_LOG_ENABLED 的生效值，并在用例结束时还原。
func setErrorLogging(t *testing.T, enabled bool) {
	t.Helper()
	raw := "false"
	if enabled {
		raw = "true"
	}
	common.SetFeatureFlagOverride(common.FlagErrorLogEnabled, &raw)
	t.Cleanup(func() { common.SetFeatureFlagOverride(common.FlagErrorLogEnabled, nil) })
}

// setConsumeLogging 覆盖消费日志开关（包级变量），并在用例结束时还原。
func setConsumeLogging(t *testing.T, enabled bool) {
	t.Helper()
	prev := common.LogConsumeEnabled
	common.LogConsumeEnabled = enabled
	t.Cleanup(func() { common.LogConsumeEnabled = prev })
}

// seedStatusLogs 写入窗口内/外的消费与错误日志，供窗口口径断言使用。
func seedStatusLogs(t *testing.T, db *gorm.DB, now int64) {
	t.Helper()
	inWindow := now - 3600
	outOfWindow := now - publicStatusWindowSeconds - 3600
	require.NoError(t, db.Create([]model.Log{
		{Type: model.LogTypeConsume, CreatedAt: inWindow, Quota: 1},
		{Type: model.LogTypeConsume, CreatedAt: inWindow, Quota: 1},
		{Type: model.LogTypeConsume, CreatedAt: inWindow, Quota: 1},
		{Type: model.LogTypeConsume, CreatedAt: outOfWindow, Quota: 1},
		{Type: model.LogTypeError, CreatedAt: inWindow},
		{Type: model.LogTypeError, CreatedAt: inWindow},
		{Type: model.LogTypeError, CreatedAt: outOfWindow},
	}).Error)
}

// 成功率的**分母口径**：只有"落了日志的请求"参与（消费日志 + 错误日志），
// 且必须随日志开关变化；任一开关关闭时对应数字与成功率都为 nil（不可用）。
func TestPublicStatusTrafficRespectsLogSwitches(t *testing.T) {
	db := setupPublicStatusLogDB(t)
	now := time.Now().Unix()
	seedStatusLogs(t, db, now)
	setConsumeLogging(t, true)

	// 错误日志关闭（默认部署形态）：请求量可得，错误数与成功率**不可用**。
	setErrorLogging(t, false)
	off := publicStatusForTest(t).Traffic
	assert.NotNil(t, off.Requests, "消费日志开启时请求量必须可得")
	assert.EqualValues(t, 3, *off.Requests, "窗口外的消费日志不得计入")
	assert.Nil(t, off.Errors, "错误日志未开启时不得给出错误数（0 会被误读成没有失败）")
	assert.Nil(t, off.SuccessRate, "错误数不可得时不得给出成功率")
	assert.False(t, off.ErrorLoggingEnabled, "响应必须如实报告错误日志开关状态")

	// 错误日志开启：错误数与成功率都得给出，且窗口外那条不计入。
	setErrorLogging(t, true)
	on := publicStatusForTest(t).Traffic
	require.NotNil(t, on.Requests)
	require.NotNil(t, on.Errors)
	assert.EqualValues(t, 3, *on.Requests)
	assert.EqualValues(t, 2, *on.Errors, "窗口外的错误日志不得计入")
	require.NotNil(t, on.SuccessRate)
	assert.InDelta(t, 3.0/5.0, *on.SuccessRate, 1e-9, "成功率 = 非错误日志 / 全部已记录日志")
	assert.EqualValues(t, publicStatusWindowSeconds, on.WindowSeconds, "统计窗口必须随响应返回")

	// 消费日志关闭：请求量与成功率不可用，错误数仍可得。
	setConsumeLogging(t, false)
	onlyErrors := publicStatusForTest(t).Traffic
	assert.Nil(t, onlyErrors.Requests, "消费日志未开启时不得给出请求量")
	assert.Nil(t, onlyErrors.SuccessRate, "缺少分母时不得给出成功率")
	require.NotNil(t, onlyErrors.Errors)
	assert.EqualValues(t, 2, *onlyErrors.Errors)
}

// 依赖探测：主库/日志库可达即 operational（同实例时标记 shared，不重复探测）；
// Redis 未配置如实报告 disabled（"没被使用"与"不可用"是两件事）。
func TestPublicStatusComponentsProbeDependencies(t *testing.T) {
	setupPublicStatusLogDB(t)
	status := publicStatusForTest(t)

	states := map[string]PublicStatusComponent{}
	for _, c := range status.Components {
		states[c.Key] = c
	}
	require.Contains(t, states, PublicStatusComponentDatabase)
	assert.Equal(t, PublicStatusStateOperational, states[PublicStatusComponentDatabase].State)

	require.Contains(t, states, PublicStatusComponentLogDatabase)
	assert.Equal(t, PublicStatusStateOperational, states[PublicStatusComponentLogDatabase].State)
	assert.True(t, states[PublicStatusComponentLogDatabase].Shared,
		"日志库与主库同实例时必须标记 shared，而不是假装分别探测过")

	require.Contains(t, states, PublicStatusComponentRedis)
	assert.Equal(t, PublicStatusStateDisabled, states[PublicStatusComponentRedis].State,
		"未配置 Redis 时应为 disabled —— 它不是 down，混淆会让状态页误报")

	// 版本与运行时长来自进程自身，必须存在。
	assert.Equal(t, common.Version, status.Version)
	assert.GreaterOrEqual(t, status.UptimeSeconds, int64(0))
	assert.GreaterOrEqual(t, status.GeneratedAt, status.StartedAt)
}

// 依赖不可用 → 整体 degraded；日志库缺失时流量字段全部为不可用，且**不得 panic**。
func TestPublicStatusDegradesWhenDependenciesMissing(t *testing.T) {
	setupPublicStatusLogDB(t)
	model.DB, model.LOG_DB = nil, nil

	status := publicStatusForTest(t)
	assert.Equal(t, PublicStatusStateDegraded, status.Status)
	assert.Nil(t, status.Traffic.Requests)
	assert.Nil(t, status.Traffic.Errors)
	assert.Nil(t, status.Traffic.SuccessRate)
}

// 后台任务状态只依据心跳时间戳：没有登记 → unknown（只是没接入观测，不是坏了）；
// 超过判定窗口 → degraded；窗口内 → operational。
func TestPublicStatusBackgroundStateFollowsHeartbeats(t *testing.T) {
	now := time.Now().Unix()

	assert.Equal(t, PublicStatusStateUnknown,
		publicStatusBackgroundState(PublicStatusBackground{}, now),
		"没有任何心跳登记时只能报 unknown")

	stale := PublicStatusBackground{
		ActivityWindowSeconds: int64(publicStatusLoopActivityWindow / time.Second),
		MostRecentRunAt:       now - int64(publicStatusLoopActivityWindow/time.Second) - 1,
		Loops:                 []PublicStatusLoop{{Name: "some_loop", LastRunAt: now - 3600}},
	}
	assert.Equal(t, PublicStatusStateDegraded, publicStatusBackgroundState(stale, now))

	stale.MostRecentRunAt = now - int64(publicStatusLoopActivityWindow/time.Second)
	assert.Equal(t, PublicStatusStateOperational, publicStatusBackgroundState(stale, now),
		"恰好等于判定窗口（未超过）应仍算在跑")

	// 真实心跳接入后的聚合视图：名字与时间戳都来自心跳表，窗口随响应返回。
	common.RecordLoopHeartbeat("public_status_contract_loop")
	background := publicStatusBackground()
	assert.Equal(t, int64(publicStatusLoopActivityWindow/time.Second), background.ActivityWindowSeconds)
	assert.GreaterOrEqual(t, background.MostRecentRunAt, now-5)
	names := make([]string, 0, len(background.Loops))
	for _, loop := range background.Loops {
		names = append(names, loop.Name)
	}
	assert.Contains(t, names, "public_status_contract_loop")
	assert.Equal(t, PublicStatusStateOperational, publicStatusBackgroundState(background, now))
}
