package training_capture

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 采集 worker 涉及**两个持久化面**：授权表（判断该不该采）
// 与水位表（判断该不该重传）。只测纯函数无法覆盖
// 「上传失败后水位有没有被错误推进」这类**跨 DB 与上传的原子性**问题，
// 而后者一旦出错就是静默丢数据。
//
// 因此这里建一个**真实 SQLite 库**（与生产同方言路径之一），
// 用与 `model.migrateDB` 相同的表定义建表，跑真实的 SQL。

var (
	testDBOnce sync.Once
	testDBErr  error
	testDBDir  string
)

// ensureTestDB 惰性建立临时 SQLite 库并指向 model.DB。
//
// 所有需要 DB 的用例共享同一个库文件（Windows 上频繁开关文件会锁冲突），
// 用例之间用**不同的用户 ID / 会话 ID** 隔离，不靠清表。
func ensureTestDB(t *testing.T) error {
	t.Helper()
	testDBOnce.Do(func() {
		// **不用 t.TempDir()**：它在用例结束时就 RemoveAll，而 SQLite 连接
		// 是进程级共享的（后续用例还要用），Windows 上会因文件被占用而
		// 让用例失败在 cleanup 阶段——那是**假失败**，会掩盖真实断言结果。
		// 用 os.MkdirTemp + 进程退出时清理，生命周期与连接一致。
		dir, err := os.MkdirTemp("", "training-capture-test-")
		if err != nil {
			testDBErr = fmt.Errorf("mkdir temp: %w", err)
			return
		}
		testDBDir = dir
		path := filepath.Join(dir, "training-capture-test.db")

		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			testDBErr = fmt.Errorf("open sqlite: %w", err)
			return
		}
		// 只建本功能需要的两张表——**用被测试的真实模型定义**，
		// 而不是在测试里另写一份 DDL（后者会与生产 schema 漂移，
		// 测试通过但生产建表失败）。
		// `SystemTaskLock` 是 worker 的多实例协调（租约）所需：worker 在消费前会
		// `AcquireTrainingCaptureLease`，缺表会让**所有**消费路径报
		// `no such table: system_task_locks` —— 表现为「worker 没消费队列」，
		// 极容易被误判成采集链路坏了。测试库必须一并建这张表。
		if err := db.AutoMigrate(
			&model.TrainingDataConsent{},
			&model.TrainingDataWatermark{},
			&model.SystemTaskLock{},
		); err != nil {
			testDBErr = fmt.Errorf("automigrate: %w", err)
			return
		}
		model.DB = db
		common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	})
	return testDBErr
}

// CleanupTestDB 关闭测试库并删除临时目录。
// 由 TestMain 在跑完所有用例后调用（Windows 上必须先关连接再删文件）。
func CleanupTestDB() {
	if model.DB == nil {
		return
	}
	if sqlDB, err := model.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	model.DB = nil
	if testDBDir != "" {
		_ = os.RemoveAll(testDBDir)
	}
}

// loadWatermarkForTest 读水位（包装 model 调用，避免测试里到处 import model）。
func loadWatermarkForTest(userId int, conversationId string) (*model.TrainingDataWatermark, error) {
	return model.GetTrainingDataWatermark(userId, conversationId)
}

// grantForTest 给用户写一条「当前版本已授权」记录，让 IsGranted 为真。
func grantForTest(t *testing.T, userId int) {
	t.Helper()
	if err := model.UpsertTrainingDataConsent(userId, true, CurrentPolicyVersion, 1791937800); err != nil {
		t.Fatalf("写入授权记录失败：%v", err)
	}
	InvalidateConsent(userId)
	// 授权缓存 TTL 30s；测试里必须能立刻看到，故再显式清一次。
	ResetConsentCacheForTest()
}

// TestMain 在全部用例跑完后关闭测试库并清理临时目录。
func TestMain(m *testing.M) {
	code := m.Run()
	CleanupTestDB()
	os.Exit(code)
}
