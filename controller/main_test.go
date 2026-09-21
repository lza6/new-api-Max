package controller

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"gorm.io/gorm"
)

// TestMain provides a shared, fully-migrated in-memory database for every
// controller test.
//
// Without it, model.DB stays uninitialized, and tests that settle tasks in
// background goroutines keep writing to the restored global DB after their
// local database swap is reverted (async teardown race). Those writes land in
// a schema-less global DB and produce "no such table" noise plus flaky
// failures (e.g. the plugin-protocol durable-barrier settlement tests).
func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open controller test db: " + err.Error())
	}
	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&model.Channel{}, &model.Token{}, &model.User{}, &model.UserSession{},
		&model.AuthFlow{}, &model.ExternalIdentityClaim{}, &model.PasskeyCredential{},
		&model.Option{}, &model.LoginEncryptionKey{}, &model.Redemption{},
		&model.RedemptionUsage{}, &model.ChannelCombo{}, &model.Ability{}, &model.Log{},
		&model.Midjourney{}, &model.TopUp{}, &model.QuotaData{}, &model.Task{},
		&model.TaskEvent{}, &model.TaskPlugin{}, &model.Model{}, &model.Vendor{},
		&model.PrefillGroup{}, &model.Setup{}, &model.TwoFA{}, &model.TwoFABackupCode{},
		&model.Checkin{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{},
		&model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{},
		&model.CustomOAuthProvider{}, &model.UserOAuthBinding{}, &model.PerfMetric{},
		&model.SystemInstance{}, &model.SystemTask{}, &model.SystemTaskLock{},
		&model.CasbinRule{}, &model.AuthzRole{}, &model.BannedIP{},
		&model.WebRequestLog{}, &model.EventDelivery{},
	); err != nil {
		panic("failed to migrate controller test db: " + err.Error())
	}

	os.Exit(m.Run())
}
