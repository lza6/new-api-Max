package service

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupBackupTestDB 建一个含最小表集的内存库，并在测试结束还原全局 DB。
func setupBackupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// 备份会遍历全部表，测试库需完整 schema。
	require.NoError(t, db.AutoMigrate(append(model.BackupTables(), model.LogTables()...)...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		_ = sqlDB.Close()
	})
	return db
}

func readGzip(t *testing.T, r io.Reader) string {
	t.Helper()
	gz, err := gzip.NewReader(r)
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()
	raw, err := io.ReadAll(gz)
	require.NoError(t, err)
	return string(raw)
}

// TestBackupRoundTrip 导出→导入往返：数据必须完整、且导入是幂等的
// （重复导入不产生重复行，这是「可安全重放」的关键契约）。
func TestBackupRoundTrip(t *testing.T) {
	db := setupBackupTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "backup-a", Password: "x", Role: 1, Group: "default", Status: 1, Quota: 100, AffCode: "aff-aaa"}).Error)
	require.NoError(t, db.Create(&model.User{Username: "backup-b", Password: "x", Role: 1, Group: "default", Status: 1, Quota: 200, AffCode: "aff-bbb"}).Error)

	var buf bytes.Buffer
	summary, err := StreamDatabaseBackup(&buf, false)
	require.NoError(t, err)
	require.Greater(t, summary.Total, int64(0))

	// 清空后导入，数据必须回来
	require.NoError(t, db.Exec("DELETE FROM users").Error)
	var count int64
	require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
	require.Equal(t, int64(0), count)

	result, err := ImportDatabaseBackup(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Inserted["users"])
	require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)

	// 幂等：再导一次，全部跳过、不新增
	result2, err := ImportDatabaseBackup(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, int64(0), result2.Inserted["users"], "重复导入不得新增行")
	assert.Equal(t, int64(2), result2.Skipped["users"])
	require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)

	// 导出内容可读（JSON Lines，含 meta 头与表段头）
	text := readGzip(t, bytes.NewReader(buf.Bytes()))
	assert.Contains(t, text, `"kind":"new-api-db-backup"`)
	assert.Contains(t, text, `"__table__":"users"`)
	assert.Contains(t, text, "backup-a")
}

// TestBackupImportRejectsGarbage 非 gzip / 非本格式文件必须被明确拒绝，
// 而不是静默「导入成功但零行」——否则用户会误以为恢复完成。
func TestBackupImportRejectsGarbage(t *testing.T) {
	setupBackupTestDB(t)

	_, err := ImportDatabaseBackup(strings.NewReader("this is not gzip"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gzip")

	// 合法 gzip 但不是本格式（缺 meta 头）
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("{\"foo\":1}\n"))
	_ = gz.Close()
	_, err = ImportDatabaseBackup(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "格式不正确")
}

// TestBackupImportHigherVersionRejected 未来版本导出的文件必须明确拒绝，
// 避免用旧代码误读新格式导致数据错乱。
func TestBackupImportHigherVersionRejected(t *testing.T) {
	setupBackupTestDB(t)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(`{"kind":"new-api-db-backup","version":999}` + "\n"))
	_ = gz.Close()

	_, err := ImportDatabaseBackup(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "版本")
}
