package service

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestBackupImportReportsNonConflictErrorsPerDatabase 三库语义一致性（审查 R1 回归）：
//
// 导入的「冲突跳过」必须**只吞主键冲突**，不得吞掉其它错误。
// 背景：MySQL 的 `INSERT IGNORE` 会把 VARCHAR 超长静默截断、把 NOT NULL 的 NULL
// 静默强转成 ”/0，导致「导入报告成功、行数正确、内容被篡改」；而 PG/SQLite 会
// 明确报错 —— 三库语义不一致（实测确认）。
//
// 因此 `insertRows` 在 MySQL 上改用 `ON DUPLICATE KEY UPDATE <pk>=<pk>`：
// 只吞主键冲突，其它错误照报。本用例锁定该契约，三库都跑（无 DSN 则 SKIP）。
func TestBackupImportReportsNonConflictErrorsPerDatabase(t *testing.T) {
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
				sqlDB, dbErr := db.DB()
				require.NoError(t, dbErr)
				sqlDB.SetMaxOpenConns(1)
			}
			// 只建 users（本用例只需一张有 PK + 非空列的表）
			require.NoError(t, db.Migrator().DropTable(&model.User{}))
			require.NoError(t, db.AutoMigrate(&model.User{}))

			prevDB, prevLog := model.DB, model.LOG_DB
			prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB = db, db
			common.SetDatabaseTypes(dbType(tc.name), dbType(tc.name))
			t.Cleanup(func() {
				model.DB, model.LOG_DB = prevDB, prevLog
				common.SetDatabaseTypes(prevMain, prevLogType)
			})

			// 1) 正常插入成功
			ins, _, err := insertRows("users", []map[string]any{
				{"id": 1, "username": "err-matrix-a", "password": "x", "role": 1, "status": 1, "aff_code": "em-aff-a"},
			})
			require.NoError(t, err, "%s: normal insert must succeed", tc.name)
			require.Equal(t, int64(1), ins)

			// 2) 主键冲突 → 必须被跳过（不报错、不重复）
			ins2, skip2, err2 := insertRows("users", []map[string]any{
				{"id": 1, "username": "err-matrix-a", "password": "x", "role": 1, "status": 1, "aff_code": "em-aff-a"},
			})
			require.NoError(t, err2, "%s: primary-key conflict must be skipped, not error", tc.name)
			require.Equal(t, int64(0), ins2, "%s: conflict must insert 0", tc.name)
			require.Equal(t, int64(1), skip2, "%s: conflict must count as skipped", tc.name)

			// 3) 非冲突错误必须报出（`password` 在本模型非空）：
			//    修复前 MySQL 的 INSERT IGNORE 会静默强转 NULL → '' 并返回 nil。
			_, _, err3 := insertRows("users", []map[string]any{
				{"id": 2, "username": "err-matrix-b", "password": nil, "role": 1, "status": 1, "aff_code": "em-aff-b"},
			})
			require.Error(t, err3, "%s: NOT NULL violation must be reported (was silently coerced before the fix)", tc.name)
		})
	}
}
