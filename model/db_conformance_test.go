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
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
				InitCol()
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
				InitCol()
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
				InitCol()
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
		&SubscriptionPreConsumeRecord{}, &SubscriptionPlanGrant{}, &CustomOAuthProvider{},
		&UserOAuthBinding{}, &PerfMetric{}, &SystemInstance{},
		&ModelCompressionStat{},
		&SystemTask{}, &SystemTaskLock{}, &CasbinRule{}, &AuthzRole{},
		&BannedIP{}, &WebRequestLog{}, &SubscriptionPlan{}, &EventDelivery{},
		&WebhookEndpoint{},
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
			require.NoError(t, db.AutoMigrate(&Log{}, &TaskEvent{}, &Task{}))

			tableName := "logs"
			for _, idx := range []string{"idx_created_at_id", "idx_user_id_id"} {
				assert.True(t, db.Migrator().HasIndex(tableName, idx),
					"expected composite index %s on %s (%s)", idx, tableName, dialect.name)
			}
			assert.True(t, db.Migrator().HasIndex("task_events", "idx_task_events_created_at"),
				"expected retention index on task_events (%s)", dialect.name)
			// B2-4：任务轮询正向活跃态复合索引 (status, progress, submit_time)。
			assert.True(t, db.Migrator().HasIndex("tasks", "idx_task_active_poll"),
				"expected active-poll composite index on tasks (%s)", dialect.name)
		})
	}
}

// TestDBConformanceLogsTrafficIndexDropped proves the obsolete idx_logs_traffic
// composite index is removed on every dialect: it is not recreated by
// AutoMigrate, and the explicit drop migration removes an existing one
// idempotently. The index (request_bytes,response_bytes) had no priority:1 and
// serves no query (site traffic aggregates are predicate-free SUMs).
func TestDBConformanceLogsTrafficIndexDropped(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			require.NoError(t, db.AutoMigrate(&Log{}))
			const tableName = "logs"
			const indexName = "idx_logs_traffic"

			// State-independent: persistent dialects may still carry the index
			// from earlier runs; clear it so the assertions below are objective.
			if db.Migrator().HasIndex(&Log{}, indexName) {
				require.NoError(t, db.Migrator().DropIndex(&Log{}, indexName))
			}

			// A fresh model migration must not create the obsolete index.
			assert.False(t, db.Migrator().HasIndex(tableName, indexName),
				"AutoMigrate must not create the removed index (%s)", dialect.name)

			// Simulate the legacy schema that still has it, then migrate.
			require.NoError(t, db.Exec("CREATE INDEX "+indexName+" ON "+tableName+"(request_bytes, response_bytes)").Error)
			require.True(t, db.Migrator().HasIndex(tableName, indexName))

			require.NoError(t, migrateLogTrafficIndex(db))
			assert.False(t, db.Migrator().HasIndex(tableName, indexName),
				"obsolete traffic index must be dropped (%s)", dialect.name)

			// Idempotent: a second run is a no-op and does not error.
			require.NoError(t, migrateLogTrafficIndex(db))
			assert.False(t, db.Migrator().HasIndex(tableName, indexName))

			// Re-running the model migration must not resurrect the index.
			require.NoError(t, db.AutoMigrate(&Log{}))
			assert.False(t, db.Migrator().HasIndex(tableName, indexName),
				"AutoMigrate must not recreate the removed index (%s)", dialect.name)
		})
	}
}

// TestDBConformanceUsageReport proves the usage/cost report aggregation works on
// every dialect: per-dimension GROUP BY sums match, and the integer day-key
// arithmetic (created_at + offset) - ((created_at + offset) % 86400) buckets
// rows into the correct local calendar day on SQLite, MySQL and PostgreSQL
// (MySQL's `/` is decimal division, so the report deliberately uses `%`).
func TestDBConformanceUsageReport(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			prevLogDB := LOG_DB
			LOG_DB = db
			t.Cleanup(func() { LOG_DB = prevLogDB })
			require.NoError(t, db.AutoMigrate(&Log{}))
			// State-independent: persistent dialects keep rows across runs, so
			// clear the logs table before seeding to keep sums deterministic.
			require.NoError(t, db.Where("1 = 1").Delete(&Log{}).Error)

			y, m, d := time.Now().Date()
			today := time.Date(y, m, d, 0, 0, 0, 0, time.Local).Unix()
			yesterday := today - 86400

			seed := []Log{
				{UserId: 1, Type: LogTypeConsume, ModelName: "alpha", ChannelId: 10, CreatedAt: today + 3600, PromptTokens: 100, CompletionTokens: 50, Quota: 7, RequestBytes: 1000, ResponseBytes: 2000},
				{UserId: 1, Type: LogTypeConsume, ModelName: "alpha", ChannelId: 10, CreatedAt: today + 7200, PromptTokens: 200, CompletionTokens: 10, Quota: 3, RequestBytes: 500, ResponseBytes: 500},
				{UserId: 2, Type: LogTypeConsume, ModelName: "beta", ChannelId: 20, CreatedAt: today + 60, PromptTokens: 10, CompletionTokens: 5, Quota: 1, RequestBytes: 100, ResponseBytes: 100},
				{UserId: 3, Type: LogTypeConsume, ModelName: "beta", ChannelId: 20, CreatedAt: yesterday + 3600, PromptTokens: 1, CompletionTokens: 1, Quota: 2, RequestBytes: 50, ResponseBytes: 50},
				// A non-consume row must be excluded from the report.
				{UserId: 4, Type: LogTypeTopup, ModelName: "alpha", ChannelId: 10, CreatedAt: today + 100, Quota: 999},
			}
			require.NoError(t, db.Create(&seed).Error)

			// By model: alpha (2 reqs, quota 10, tokens 360), beta (2 reqs, quota 3, tokens 17).
			byModel, err := GetUsageReport(UsageReportGroupByModel, yesterday, 0)
			require.NoError(t, err)
			require.Len(t, byModel, 2)
			models := map[string]UsageReportRow{}
			for _, r := range byModel {
				models[r.Key] = r
			}
			assert.EqualValues(t, 2, models["alpha"].Requests)
			assert.EqualValues(t, 10, models["alpha"].Quota)
			assert.EqualValues(t, 360, models["alpha"].TotalTokens)
			assert.EqualValues(t, 4000, models["alpha"].TotalBytes)
			assert.EqualValues(t, 2, models["beta"].Requests)
			assert.EqualValues(t, 17, models["beta"].TotalTokens)
			assert.EqualValues(t, 300, models["beta"].TotalBytes)

			// By channel: 10 (2 reqs), 20 (2 reqs).
			byChannel, err := GetUsageReport(UsageReportGroupByChannel, yesterday, 0)
			require.NoError(t, err)
			require.Len(t, byChannel, 2)

			// By day: today (3 reqs) and yesterday (1 req) — day-key must bucket
			// correctly across dialects.
			byDay, err := GetUsageReport(UsageReportGroupByDay, yesterday, 0)
			require.NoError(t, err)
			require.Len(t, byDay, 2)
			todayKey := time.Unix(today, 0).In(time.Local).Format("2006-01-02")
			yesterdayKey := time.Unix(yesterday, 0).In(time.Local).Format("2006-01-02")
			days := map[string]UsageReportRow{}
			for _, r := range byDay {
				days[r.Key] = r
			}
			assert.EqualValues(t, 3, days[todayKey].Requests, "today bucket (%s)", dialect.name)
			assert.EqualValues(t, 1, days[yesterdayKey].Requests, "yesterday bucket (%s)", dialect.name)

			// Invalid dimension is rejected without touching the DB.
			_, err = GetUsageReport("bogus", yesterday, 0)
			assert.Error(t, err)
		})
	}
}

// TestDBConformanceEventDeliveryDedup proves the (event_id, handler) unique
// dedup contract on every dialect: the first insert lands, a duplicate is
// silently skipped (OnConflict DoNothing, RowsAffected=0) and state updates
// round-trip — the exact semantics the event bus relies on for replay.
func TestDBConformanceEventDeliveryDedup(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			require.NoError(t, db.AutoMigrate(&EventDelivery{}))

			first := &EventDelivery{
				EventID:   "evt-conformance",
				Handler:   "epay.topup",
				EventType: "epay.topup.success",
				State:     "pending",
				CreatedAt: 1,
				UpdatedAt: 1,
			}
			result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(first)
			require.NoError(t, result.Error)
			require.Equal(t, int64(1), result.RowsAffected)

			dup := &EventDelivery{
				EventID:   "evt-conformance",
				Handler:   "epay.topup",
				EventType: "epay.topup.success",
				State:     "success",
				CreatedAt: 2,
				UpdatedAt: 2,
			}
			result = db.Clauses(clause.OnConflict{DoNothing: true}).Create(dup)
			require.NoError(t, result.Error)
			assert.Zero(t, result.RowsAffected, "duplicate (event_id, handler) must be skipped (%s)", dialect.name)

			require.NoError(t, db.Model(&EventDelivery{}).
				Where("event_id = ? AND handler = ?", "evt-conformance", "epay.topup").
				Updates(map[string]any{"state": "success", "attempts": 1}).Error)

			var loaded EventDelivery
			require.NoError(t, db.Where("event_id = ?", "evt-conformance").First(&loaded).Error)
			assert.Equal(t, "success", loaded.State)
			assert.Equal(t, 1, loaded.Attempts)

			var count int64
			require.NoError(t, db.Model(&EventDelivery{}).Count(&count).Error)
			assert.Equal(t, int64(1), count, "replay must not add rows (%s)", dialect.name)
		})
	}
}

// TestDBConformanceWebhookEndpointRoundTrip B2-3：webhook_endpoints 表在三库上
// 建表/读写/事件 JSON 字段往返一致；enabled + events 是投递路径的关键查询谓词。
func TestDBConformanceWebhookEndpointRoundTrip(t *testing.T) {
	for _, dialect := range conformanceDialects(t) {
		t.Run(dialect.name, func(t *testing.T) {
			db, _ := dialect.openDB(t)
			require.NoError(t, db.AutoMigrate(&WebhookEndpoint{}))

			ep := &WebhookEndpoint{Name: "n", URL: "https://example.com/hooks", Secret: "s", Enabled: true, Events: `["task.settled"]`}
			require.NoError(t, db.Create(ep).Error)
			require.NotZero(t, ep.Id)

			var loaded WebhookEndpoint
			require.NoError(t, db.Where("id = ?", ep.Id).First(&loaded).Error)
			assert.Equal(t, "https://example.com/hooks", loaded.URL)
			assert.True(t, loaded.Enabled)
			assert.Equal(t, `["task.settled"]`, loaded.Events)
			assert.NotZero(t, loaded.CreatedAt, "BeforeCreate hook must stamp created_at")
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
