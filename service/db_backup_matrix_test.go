package service

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestBackupCrossDatabaseMatrix 三库兼容性（AGENTS 硬要求）：
// db_backup 用 clause.OnConflict{DoNothing:true} 落库，GORM 会为
// PG/SQLite 生成 ON CONFLICT DO NOTHING、为 MySQL 生成 INSERT IGNORE。
// 本用例在真实 SQLite/MySQL/PostgreSQL 上验证「导出→导入→幂等重放」，
// 确认三库语义一致（MySQL 若无 DSN 则 SKIP，不伪造通过）。
func TestBackupCrossDatabaseMatrix(t *testing.T) {
	type target struct {
		name, dsn string
		open      func(string) gorm.Dialector
	}
	targets := []target{
		{name: "sqlite", open: func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{name: "mysql", dsn: os.Getenv("TEST_MYSQL_DSN"), open: func(d string) gorm.Dialector { return mysql.Open(d) }},
		{name: "postgres", dsn: os.Getenv("TEST_POSTGRES_DSN"), open: func(d string) gorm.Dialector { return postgres.Open(d) }},
	}
	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name != "sqlite" && tc.dsn == "" {
				t.Skipf("no DSN for %s", tc.name)
			}
			db, err := gorm.Open(tc.open(tc.dsn), &gorm.Config{})
			require.NoError(t, err)
			if tc.name == "sqlite" {
				// :memory: 每个连接是独立库 —— 必须固定单连接，否则导出/导入
				// 会落在不同连接的不同库上（仓库既有纪律 B-2）。
				sqlDB, dbErr := db.DB()
				require.NoError(t, dbErr)
				sqlDB.SetMaxOpenConns(1)
			}
			_ = db.Migrator().DropTable(model.BackupTables()...)
			require.NoError(t, db.AutoMigrate(model.BackupTables()...))

			prevDB, prevLog := model.DB, model.LOG_DB
			prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB = db, db
			common.SetDatabaseTypes(dbType(tc.name), dbType(tc.name))
			t.Cleanup(func() {
				model.DB, model.LOG_DB = prevDB, prevLog
				common.SetDatabaseTypes(prevMain, prevLogType)
			})

			require.NoError(t, db.Create(&model.User{Username: "xdb-a", Password: "x", Role: 1, Group: "default", Status: 1, Quota: 7, AffCode: "xdb-aff-a"}).Error)
			require.NoError(t, db.Create(&model.User{Username: "xdb-b", Password: "x", Role: 1, Group: "default", Status: 1, Quota: 8, AffCode: "xdb-aff-b"}).Error)

			var buf bytes.Buffer
			_, err = StreamDatabaseBackup(&buf, false)
			require.NoError(t, err, "export on %s", tc.name)
			// 导出必须真的包含数据（压缩流需解压后再校验，否则永远 false）。
			gz, gzErr := gzip.NewReader(bytes.NewReader(buf.Bytes()))
			require.NoError(t, gzErr)
			rawTxt, _ := io.ReadAll(gz)
			require.Contains(t, string(rawTxt), `"__table__":"users"`, "backup must contain users header")
			require.Contains(t, string(rawTxt), "xdb-a", "backup must contain the seeded row")

			// User 是**软删除**模型（DeletedAt）：普通 Delete 只置 deleted_at，
			// 行仍在 → 导入时 UNIQUE(username) 冲突被跳过。测试要用 Unscoped
			// 真正物理删除，才能验证「缺失行会被插入」。
			require.NoError(t, db.Unscoped().Where("username LIKE ?", "xdb-%").Delete(&model.User{}).Error)
			var n int64
			require.NoError(t, db.Unscoped().Model(&model.User{}).Where("username LIKE ?", "xdb-%").Count(&n).Error)
			require.Zero(t, n, "rows must be physically gone before import")

			res, err := ImportDatabaseBackup(bytes.NewReader(buf.Bytes()))
			require.NoError(t, err, "import on %s", tc.name)
			assert.Equal(t, int64(2), res.Inserted["users"], "first import on %s", tc.name)

			// 幂等重放：冲突必须被正确跳过（这是 ON CONFLICT DO NOTHING / INSERT IGNORE 三库一致性）
			res2, err := ImportDatabaseBackup(bytes.NewReader(buf.Bytes()))
			require.NoError(t, err, "replay on %s", tc.name)
			assert.Zero(t, res2.Inserted["users"], "replay must insert 0 on %s", tc.name)
			assert.Empty(t, res2.Errors, "replay must not error on %s", tc.name)

			require.NoError(t, db.Unscoped().Model(&model.User{}).Where("username LIKE ?", "xdb-%").Count(&n).Error)
			assert.Equal(t, int64(2), n, "row count stable on %s", tc.name)
		})
	}
}

func dbType(name string) common.DatabaseType {
	switch name {
	case "mysql":
		return common.DatabaseTypeMySQL
	case "postgres":
		return common.DatabaseTypePostgreSQL
	default:
		return common.DatabaseTypeSQLite
	}
}
