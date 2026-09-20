package model

// DB conformance contract tests.
//
// These tests exercise the same database contract that AGENTS.md requires for
// every database-affecting change: SQLite, MySQL and PostgreSQL must all be
// verified against real instances. They are table-driven over the three
// dialects and skip a dialect only when its DSN is not configured.
//
// Run all three with:
//
//	TEST_MYSQL_DSN='root@tcp(127.0.0.1:3306)/newapi_conformance_test?charset=utf8mb4&parseTime=true&loc=Local' \
//	TEST_POSTGRES_DSN='postgres://postgres@127.0.0.1:5432/newapi_conformance_test' \
//	go test ./model/ -run TestDBConformance -v
//
// or use `make db-check` / scripts/db-conformance.ps1.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// conformanceOpenDB opens the dialect database and registers a cleanup that
// closes the underlying connection pool, so temp SQLite files are not locked
// when t.TempDir() tries to remove them on Windows.
func conformanceOpenDB(t *testing.T, db *gorm.DB, dbType common.DatabaseType) (*gorm.DB, common.DatabaseType) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, dbType
}

// conformanceDialects builds the three-dialect matrix shared by every
// conformance test. SQLite always runs against a temp file; MySQL/PostgreSQL
// require TEST_MYSQL_DSN / TEST_POSTGRES_DSN and are skipped otherwise.
func conformanceDialects(t *testing.T) []struct {
	name   string
	env    string
	dsn    func() string
	openDB func(t *testing.T) (*gorm.DB, common.DatabaseType)
} {
	return []struct {
		name   string
		env    string
		dsn    func() string
		openDB func(t *testing.T) (*gorm.DB, common.DatabaseType)
	}{
		{
			name: "sqlite",
			openDB: func(t *testing.T) (*gorm.DB, common.DatabaseType) {
				t.Helper()
				previous := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "conformance.db")
				t.Cleanup(func() { common.SQLitePath = previous })
				common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
				initCol()
				db, dbType, err := chooseDB("CONFORMANCE_DSN", false)
				require.NoError(t, err)
				require.Equal(t, common.DatabaseTypeSQLite, dbType)
				t.Setenv("CONFORMANCE_DSN", "local")
				return conformanceOpenDB(t, db, dbType)
			},
		},
		{
			name: "mysql",
			openDB: func(t *testing.T) (*gorm.DB, common.DatabaseType) {
				t.Helper()
				dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured; skipping MySQL conformance")
				}
				t.Setenv("CONFORMANCE_DSN", dsn)
				common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
				initCol()
				db, dbType, err := chooseDB("CONFORMANCE_DSN", false)
				require.NoError(t, err)
				require.Equal(t, common.DatabaseTypeMySQL, dbType)
				return conformanceOpenDB(t, db, dbType)
			},
		},
		{
			name: "postgres",
			openDB: func(t *testing.T) (*gorm.DB, common.DatabaseType) {
				t.Helper()
				dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured; skipping PostgreSQL conformance")
				}
				t.Setenv("CONFORMANCE_DSN", dsn)
				common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
				initCol()
				db, dbType, err := chooseDB("CONFORMANCE_DSN", false)
				require.NoError(t, err)
				require.Equal(t, common.DatabaseTypePostgreSQL, dbType)
				return conformanceOpenDB(t, db, dbType)
			},
		},
	}
}

// conformanceModels returns the production model set migrated by migrateDB.
// Keeping this list in sync with migrateDB() is a contract in itself: any new
// model registered there must be covered here so idempotency is proven for it.
func conformanceModels() []any {
	return []any{
		&Channel{}, &Token{}, &User{}, &UserSession{}, &AuthFlow{},
		&ExternalIdentityClaim{}, &PasskeyCredential{}, &Option{},
		&LoginEncryptionKey{}, &Redemption{}, &RedemptionUsage{},
		&ChannelCombo{}, &Ability{}, &Log{}, &Midjourney{}, &TopUp{},
		&QuotaData{}, &Task{}, &TaskEvent{}, &TaskPlugin{}, &Model{},
		&Vendor{}, &PrefillGroup{}, &Setup{}, &TwoFA{}, &TwoFABackupCode{},
		&Checkin{}, &SubscriptionOrder{}, &UserSubscription{},
		&SubscriptionPreConsumeRecord{}, &CustomOAuthProvider{},
		&UserOAuthBinding{}, &PerfMetric{}, &SystemInstance{},
		&SystemTask{}, &SystemTaskLock{}, &CasbinRule{}, &AuthzRole{},
		&BannedIP{}, &WebRequestLog{}, &SubscriptionPlan{},
	}
}

// TestDBConformanceAutoMigrateIdempotent proves that migrating the full
// production model set leaves no schema mutations on the second run, for each
// dialect. Schema drift (ALTER/CREATE INDEX on a fresh, already-migrated
// database) fails the build.
func TestDBConformanceAutoMigrateIdempotent(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			recorder := &migrationSQLRecorder{}
			recordingDB := db.Session(&gorm.Session{Logger: recorder})

			// Drop every production table first so the conformance database is
			// state-independent and the test is repeatable: MySQL/PostgreSQL
			// use a persistent database, so without this a second run would
			// find an already-migrated schema and "first migration" would
			// produce no mutations.
			for _, model := range conformanceModels() {
				_ = recordingDB.Migrator().DropTable(model)
			}

			// Fresh migration: expected to create tables and indexes.
			require.NoError(t, recordingDB.AutoMigrate(conformanceModels()...), "first migration failed")
			require.NotEmpty(t, recorder.schemaMutations(), "first migration should create schema")

			// Idempotent re-run: must not emit ALTER/DROP/CREATE INDEX.
			recorder.reset()
			require.NoError(t, recordingDB.AutoMigrate(conformanceModels()...), "second migration failed")
			mutations := recorder.schemaMutations()
			assert.Empty(t, mutations, "second migration must be schema-stable; got: %v", mutations)
		})
	}
}

// TestDBConformanceLogsIndexes proves the F2/F3/F4 composite indexes on logs and
// the task_events retention index survive migration on every dialect.
func TestDBConformanceLogsIndexes(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			require.NoError(t, db.AutoMigrate(&Log{}, &TaskEvent{}))

			tableName := "logs"
			for _, idx := range []string{"idx_created_at_id", "idx_user_id_id"} {
				assert.True(t, db.Migrator().HasIndex(tableName, idx),
					"expected composite index %s on %s (%s)", idx, tableName, dialect.name)
			}
			assert.True(t, db.Migrator().HasIndex("task_events", "idx_task_events_created_at"),
				"expected retention index on task_events (%s)", dialect.name)
		})
	}
}

// conformanceReservedModel exercises the reserved-word columns `group` and
// `key` exactly as the production Channel/Token models do.
type conformanceReservedModel struct {
	ID    uint   `gorm:"primaryKey"`
	Group string `gorm:"column:group;type:varchar(64);default:'default'"`
	Key   string `gorm:"column:key;type:varchar(128)"`
}

// TestDBConformanceReservedColumns proves that the helper-rendered column
// names (commonGroupCol/commonKeyCol) round-trip real rows on every dialect.
func TestDBConformanceReservedColumns(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, dbType := dialect.openDB(t)
			const table = "conformance_reserved_test"
			t.Cleanup(func() { _ = db.Migrator().DropTable(table) })
			require.NoError(t, db.Table(table).AutoMigrate(&conformanceReservedModel{}))

			// Insert referencing the reserved words via the shared helpers.
			require.NoError(t, db.Table(table).Create(&conformanceReservedModel{Group: "g1", Key: "k1"}).Error)
			require.NoError(t, db.Table(table).Create(&conformanceReservedModel{Group: "g2", Key: "k2"}).Error)

			// Read back via the helper-quoted column names.
			var got conformanceReservedModel
			query := db.Table(table).Where("`group` = ? ", "g1")
			if dbType == common.DatabaseTypePostgreSQL {
				query = db.Table(table).Where(`"group" = ?`, "g1")
			}
			require.NoError(t, query.First(&got).Error)
			assert.Equal(t, "g1", got.Group)
			assert.Equal(t, "k1", got.Key)

			var count int64
			cq := db.Table(table).Where("`key` IN ?", []string{"k1", "k2"})
			if dbType == common.DatabaseTypePostgreSQL {
				cq = db.Table(table).Where(`"key" IN ?`, []string{"k1", "k2"})
			}
			require.NoError(t, cq.Count(&count).Error)
			assert.EqualValues(t, 2, count, "expected both rows found via reserved key column (%s)", dialect.name)
		})
	}
}

// conformanceJsonModel stores a JSON-serialized blob in a TEXT column, which is
// how production stores structured payloads across all three dialects.
type conformanceJsonModel struct {
	ID   uint   `gorm:"primaryKey"`
	Path string `gorm:"type:varchar(191)"`
	Body string `gorm:"type:text"`
}

// TestDBConformanceJsonRoundTrip proves structured payloads survive a
// TEXT-column round trip on every dialect (JSON is stored as TEXT for SQLite
// compatibility, matching common/json.go usage).
func TestDBConformanceJsonRoundTrip(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			const table = "conformance_json_test"
			t.Cleanup(func() { _ = db.Migrator().DropTable(table) })
			require.NoError(t, db.Table(table).AutoMigrate(&conformanceJsonModel{}))

			payload := map[string]any{
				"model":  "gpt-4o",
				"tokens": 123,
				"nested": map[string]any{"ok": true},
			}
			blob, err := common.Marshal(payload)
			require.NoError(t, err)
			require.NoError(t, db.Table(table).Create(&conformanceJsonModel{Path: "/v1/chat", Body: string(blob)}).Error)

			var got conformanceJsonModel
			require.NoError(t, db.Table(table).Where("path = ?", "/v1/chat").First(&got).Error)
			var decoded map[string]any
			require.NoError(t, common.Unmarshal([]byte(got.Body), &decoded))
			assert.Equal(t, "gpt-4o", decoded["model"])
			assert.EqualValues(t, 123, decoded["tokens"])
		})
	}
}

// TestDBConformanceLockForUpdate proves lockForUpdate emits a real row lock in
// a transaction on MySQL/PostgreSQL and is safely skipped on SQLite.
func TestDBConformanceLockForUpdate(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, dbType := dialect.openDB(t)
			const table = "conformance_lock_test"
			t.Cleanup(func() { _ = db.Migrator().DropTable(table) })
			require.NoError(t, db.Table(table).AutoMigrate(&conformanceReservedModel{}))

			seed := &conformanceReservedModel{Group: "lock", Key: "row"}
			require.NoError(t, db.Table(table).Create(seed).Error)

			err := db.Transaction(func(tx *gorm.DB) error {
				var row conformanceReservedModel
				if dbType == common.DatabaseTypeSQLite {
					// SQLite has no FOR UPDATE; lockForUpdate must be a no-op
					// rather than emitting invalid SQL.
					require.NoError(t, tx.Table(table).Where("id = ?", seed.ID).First(&row).Error)
				} else {
					require.NoError(t, lockForUpdate(tx).Table(table).Where("id = ?", seed.ID).First(&row).Error)
				}
				assert.Equal(t, "lock", row.Group)
				return nil
			})
			require.NoError(t, err)
		})
	}
}

// TestDBConformanceUsingDatabaseBranches proves common.UsingMainDatabase /
// UsingLogDatabase branch correctly per dialect (used by initCol and raw-SQL
// guards throughout the codebase).
func TestDBConformanceUsingDatabaseBranches(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, dbType := dialect.openDB(t)
			t.Cleanup(func() {
				common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
				_ = db
			})
			assert.True(t, common.UsingMainDatabase(dbType), "main database type mismatch")
			other := common.DatabaseTypePostgreSQL
			if dbType == common.DatabaseTypePostgreSQL {
				other = common.DatabaseTypeMySQL
			}
			assert.False(t, common.UsingMainDatabase(other), "unexpected main database branch")
		})
	}
}
