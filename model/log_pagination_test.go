package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLogPaginationDB 建一个内存 SQLite 日志库，供 B2-4 分页/聚合测试复用。
func setupLogPaginationDB(t *testing.T) *gorm.DB {
	t.Helper()
	origDB, origLogDB := DB, LOG_DB
	origMain, origLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = origDB, origLogDB
		common.SetDatabaseTypes(origMain, origLog)
	})
	return db
}

// TestLogPaginationDeferredJoinCorrectness B2-4：深分页（偏移 ≥ 阈值）改走延迟关联后，
// 分页结果必须与浅分页口径完全一致——同样的顺序、同样的 total、同样的页边界。
func TestLogPaginationDeferredJoinCorrectness(t *testing.T) {
	db := setupLogPaginationDB(t)

	// 2000 行，id 1..2000，created_at 递增。
	const total = 2000
	logs := make([]*Log, 0, total)
	for i := 0; i < total; i++ {
		logs = append(logs, &Log{
			UserId:    1,
			Type:      LogTypeConsume,
			ModelName: "m",
			CreatedAt: int64(1000 + i),
			Quota:     i,
		})
	}
	require.NoError(t, db.CreateInBatches(&logs, 200).Error)

	// 第 1 页（偏移 0，浅路径）
	page1, count1, err := GetUserLogs(1, LogTypeConsume, 0, 0, "", "", 0, 10, "", "", "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(total), count1)
	require.Len(t, page1, 10)

	// 深页：偏移 1500（≥ logDeepOffsetThreshold，走延迟关联）。
	page151, count151, err := GetUserLogs(1, LogTypeConsume, 0, 0, "", "", 1500, 10, "", "", "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(total), count151)
	require.Len(t, page151, 10, "deep page must return a full page")

	// 顺序：GetUserLogs 按数据库 id desc。日志 quota 依次为 0..1999，DB id=quota+1。
	// 偏移 0 → 末 10 条（quota 1999..1990）；偏移 1500 → quota 499..490。
	// 注：返回的 Id 字段是「显示 id」（assignDisplayLogIds 覆写为 offset+i+1），
	// 故这里用 Quota 校验真实行序。
	assert.Equal(t, 1999, page1[0].Quota)
	assert.Equal(t, 1990, page1[9].Quota)
	assert.Equal(t, 499, page151[0].Quota, "deep-offset page must match offset semantics")
	assert.Equal(t, 490, page151[9].Quota)
	assert.Equal(t, 1501, page151[0].Id, "display id must remain offset-based")

	// 边界：越界偏移返回空页、total 不变。
	pageOOB, _, err := GetUserLogs(1, LogTypeConsume, 0, 0, "", "", 5000, 10, "", "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, pageOOB)
}

// TestLogPaginationShallowPathUnchanged B2-4：浅分页（偏移 < 阈值）保持原 LIMIT/OFFSET
// 语义，结果与延迟关联路径一致（回归护栏）。
func TestLogPaginationShallowPathUnchanged(t *testing.T) {
	db := setupLogPaginationDB(t)
	logs := make([]*Log, 0, 100)
	for i := 0; i < 100; i++ {
		logs = append(logs, &Log{UserId: 7, Type: LogTypeConsume, ModelName: "m", CreatedAt: int64(100 + i)})
	}
	require.NoError(t, db.CreateInBatches(&logs, 50).Error)

	page, count, err := GetUserLogs(7, LogTypeConsume, 0, 0, "", "", 20, 10, "", "", "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(100), count)
	require.Len(t, page, 10)
	// 偏移 20 → 第 21..30 条（DB id desc，第 1 条 id=100）。显示 id = 21..30，ShallowPath 用 Id 校验显示口径。
	assert.Equal(t, 21, page[0].Id)
}

// 首字/耗时筛选：min_use_time 只返回耗时 ≥ 阈值的行（三库语义一致）。
func TestGetUserLogsMinUseTimeFilter(t *testing.T) {
	db := setupLogPaginationDB(t)
	logs := []*Log{
		{UserId: 42, Type: LogTypeConsume, ModelName: "m", CreatedAt: 100, UseTime: 5},
		{UserId: 42, Type: LogTypeConsume, ModelName: "m", CreatedAt: 101, UseTime: 25},
		{UserId: 42, Type: LogTypeConsume, ModelName: "m", CreatedAt: 102, UseTime: 180},
	}
	require.NoError(t, db.CreateInBatches(&logs, 10).Error)

	// 无筛选 → 3 条。
	_, total, err := GetUserLogs(42, LogTypeConsume, 0, 0, "", "", 0, 20, "", "", "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)

	// min_use_time=20 → 2 条（25s、180s）。
	rows, total, err := GetUserLogs(42, LogTypeConsume, 0, 0, "", "", 0, 20, "", "", "", 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.GreaterOrEqual(t, r.UseTime, 20)
	}
}
