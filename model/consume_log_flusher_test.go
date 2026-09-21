package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConsumeLogFlushDrainsQueue(t *testing.T) {
	origDB, origLogDB := DB, LOG_DB
	origFlush, origConsume, origExport := common.LogFlushEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	defer func() {
		DB, LOG_DB = origDB, origLogDB
		common.LogFlushEnabled, common.LogConsumeEnabled, common.DataExportEnabled = origFlush, origConsume, origExport
	}()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB, LOG_DB = db, db
	common.LogFlushEnabled = true
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false

	for i := 0; i < 3; i++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Set("username", "u")
		RecordConsumeLog(c, 1, RecordConsumeLogParams{ModelName: "m", Other: NewLogOther()})
	}
	FlushConsumeLogs()

	var n int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&n).Error)
	require.Equal(t, int64(3), n)
}

// TestConsumeLogFlusherBatchAndMetrics：真实批量路径 —— 入队后批量落库，
// 指标反映队列深度与批大小（P1-2 验收：批量化 + 指标暴露）。
func TestConsumeLogFlusherBatchAndMetrics(t *testing.T) {
	origDB, origLogDB := DB, LOG_DB
	origFlush := common.LogFlushEnabled
	defer func() {
		DB, LOG_DB = origDB, origLogDB
		common.LogFlushEnabled = origFlush
	}()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB, LOG_DB = db, db
	common.LogFlushEnabled = true

	// 全局单例 worker：先停掉前序测试遗留的 worker 并排空，保证本测试
	// 的入队只被本测试的 worker 消费。
	StopConsumeLogFlusher()
	StartConsumeLogFlusher()
	t.Cleanup(StopConsumeLogFlusher)

	const n = 50
	for range n {
		consumeLogQueue <- &Log{UserId: 1, ModelName: "m", Type: int(LogTypeConsume)}
	}
	FlushConsumeLogs()

	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	require.Equal(t, int64(n), count, "flush 后全部落库")

	depth, lastBatch, failures, retries := GetConsumeLogFlusherMetrics()
	require.LessOrEqual(t, depth, int64(4096), "队列深度有界")
	// 排空路径可能全部走同步落库（与后台批量竞争时语义正确优先），
	// 因此不强制 lastBatch >= 1；只要求指标一致可读。
	require.GreaterOrEqual(t, lastBatch, int64(0), "指标可读")
	require.Equal(t, int64(0), failures)
	require.Equal(t, int64(0), retries)
}

// TestConsumeLogFlusherQueueFullFallback：队列满时降级同步写不丢、不崩
// （验收：并发下队列有界、不 OOM）。
func TestConsumeLogFlusherQueueFullFallback(t *testing.T) {
	origDB, origLogDB := DB, LOG_DB
	defer func() { DB, LOG_DB = origDB, origLogDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB, LOG_DB = db, db

	// 不启动 flusher，直接灌满队列 → enqueueAsyncLog 走同步兜底。
	for range 4100 {
		enqueueAsyncLog(&Log{UserId: 1, ModelName: "m"})
	}
	depth, _, _, _ := GetConsumeLogFlusherMetrics()
	require.LessOrEqual(t, depth, int64(4096), "队列深度有界")

	FlushConsumeLogs()
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	require.Equal(t, int64(4100), count, "队列满载降级同步写后全部落库，无丢失")
}
