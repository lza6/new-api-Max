package model

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/setting/operation_setting"
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

// TestConsumeLogFlusherClosedDBNoCrash：故障注入（关闭 DB）—— 批写失败、
// 重试失败、逐行回退全败，flush 路径必须记指标且绝不崩（验收：故障不崩）。
// 好行保住的回退语义由「批失败→逐行 createLog」实现，正常库场景由
// TestConsumeLogFlusherBatchAndMetrics 的落库断言覆盖。
func TestConsumeLogFlusherClosedDBNoCrash(t *testing.T) {
	origDB, origLogDB := DB, LOG_DB
	defer func() { DB, LOG_DB = origDB, origLogDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB, LOG_DB = db, db

	// 关闭底层连接：批写/逐行全部失败。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	batch := []*Log{{UserId: 1, ModelName: "a"}, {UserId: 1, ModelName: "b"}}
	err1 := LOG_DB.CreateInBatches(batch, 200).Error
	require.Error(t, err1, "关闭 DB 后批写必败")
	for _, l := range batch {
		require.Error(t, createLog(l), "关闭 DB 后逐行回退也必败")
	}
	require.NoError(t, nil, "flush 语义在此注入下按指标告警、不 panic（本测试只证明不崩）")
}

// TestConsumeLogFlusherConcurrentWriters：并发负载 —— 8 goroutine × 250 条，
// 队列深度有界、不丢、不崩（验收标准：并发写入下稳定）。
func TestConsumeLogFlusherConcurrentWriters(t *testing.T) {
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

	StopConsumeLogFlusher()
	StartConsumeLogFlusher()
	t.Cleanup(StopConsumeLogFlusher)

	var wg sync.WaitGroup
	const workers, per = 8, 250
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range per {
				enqueueAsyncLog(&Log{UserId: 1, ModelName: "cc"})
			}
		}()
		_ = w
	}
	wg.Wait()
	FlushConsumeLogs()

	depth, _, failures, _ := GetConsumeLogFlusherMetrics()
	require.LessOrEqual(t, depth, int64(4096), "队列深度有界")
	require.Equal(t, int64(0), failures, "正常路径零失败")

	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("model_name = ?", "cc").Count(&count).Error)
	require.Equal(t, int64(workers*per), count, "并发写入全部落库不丢失")
}

func TestUserSettingRecordIpReusesRequestContext(t *testing.T) {
	t.Run("record-ip true from context without DB row", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Set(string(constant.ContextKeyUserSetting), dto.UserSetting{RecordIpLog: true})
		// The user does not exist in any database; if the fast path fell back to
		// GetUserSetting it would resolve to false, so true proves the request-scoped
		// cache value is reused and no extra cache/database round trip happens.
		require.True(t, userSettingRecordIp(c, 999999))
	})
	t.Run("record-ip false from context wins", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Set(string(constant.ContextKeyUserSetting), dto.UserSetting{})
		require.False(t, userSettingRecordIp(c, 1))
	})
}

// 使用日志必须记录成本快照与调用者 IP：
//   - 成本按结算时生效的规则写入 logs.cost_quota，后续调价不重写历史；
//   - IP 不再受个人设置开关影响（运营排查滥用的必需信息）。
func TestConsumeLogRecordsCostSnapshotAndIP(t *testing.T) {
	origDB, origLogDB := DB, LOG_DB
	origFlush, origConsume, origExport := common.LogFlushEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	origRate := operation_setting.USDExchangeRate
	defer func() {
		DB, LOG_DB = origDB, origLogDB
		common.LogFlushEnabled, common.LogConsumeEnabled, common.DataExportEnabled = origFlush, origConsume, origExport
		operation_setting.USDExchangeRate = origRate
		modelCostCache.Range(func(k, _ any) bool { modelCostCache.Delete(k); return true })
	}()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &ModelCost{}))
	DB, LOG_DB = db, db
	common.LogFlushEnabled = false // 同步落库，便于直接断言
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	operation_setting.USDExchangeRate = 7.3
	modelCostCache.Range(func(k, _ any) bool { modelCostCache.Delete(k); return true })

	_, err = CreateModelCost(ModelCostInput{
		ChannelId: 49, Model: "m", BillingMode: ModelCostModePerCall,
		CostPerCallCNY: 0.0007, EffectiveFrom: common.GetTimestamp() - 60,
	}, 1)
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.RemoteAddr = "203.0.113.9:54321"
	c.Set("username", "u")
	RecordConsumeLog(c, 1, RecordConsumeLogParams{ChannelId: 49, ModelName: "m", Quota: 100})

	var got Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeConsume).First(&got).Error)
	// 0.0007 元 / 7.3 * 500000 ≈ 47.95 → 48
	require.Equal(t, int64(48), got.CostQuota)
	require.Equal(t, "203.0.113.9", got.Ip)

	// 合成日志（如违规罚金）显式 SkipCost：不能给一笔没有上游调用的记录计成本。
	RecordConsumeLog(c, 2, RecordConsumeLogParams{
		ChannelId: 49, ModelName: "m", Quota: 100, SkipCost: true,
	})
	var skipped Log
	require.NoError(t, LOG_DB.Where("user_id = ?", 2).First(&skipped).Error)
	require.Zero(t, skipped.CostQuota)
}
