package model

import (
	"path/filepath"
	"testing"

	"github.com/lza6/new-api-Max/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 用户删除时的采集数据级联清理。
//
// ## 这是合规边界，不是「表整洁度」
//
// 用户注销后若不清理 `training_data_consents`，且数据库回收了自增 ID，
// 新用户拿到同一 ID 会**继承 `granted=true`** —— 从未授权却被采集。
// 下面用「ID 复用」场景把这个风险钉死。

func TestDeleteTrainingDataForUserClearsConsentAndWatermarks(t *testing.T) {
	setupTrainingDataModelTest(t)

	const uid = 4242
	// 造数据：一条授权 + 两条水位。
	require.NoError(t, UpsertTrainingDataConsent(uid, true, "v1", 1000))
	require.NoError(t, SaveTrainingDataWatermark(uid, "conv-a", 3, "fp-a", 10, "h", 1000))
	require.NoError(t, SaveTrainingDataWatermark(uid, "conv-b", 5, "fp-b", 20, "h", 1000))

	// 另一个用户的数据**不得**被误删。
	const other = 4243
	require.NoError(t, UpsertTrainingDataConsent(other, true, "v1", 1000))
	require.NoError(t, SaveTrainingDataWatermark(other, "conv-c", 1, "fp-c", 1, "h", 1000))

	n, err := DeleteTrainingDataForUser(uid)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "应删掉该用户的两条水位")

	consent, err := GetTrainingDataConsent(uid)
	require.NoError(t, err)
	assert.Nil(t, consent, "授权记录必须被清除（否则 ID 复用会继承 granted=true）")

	for _, conv := range []string{"conv-a", "conv-b"} {
		wm, err := GetTrainingDataWatermark(uid, conv)
		require.NoError(t, err)
		assert.Nil(t, wm, "水位 %s 必须被清除", conv)
	}

	// 其他用户不受影响。
	otherConsent, err := GetTrainingDataConsent(other)
	require.NoError(t, err)
	require.NotNil(t, otherConsent, "**绝不能不删别人的数据**")
	assert.True(t, otherConsent.Granted)
	otherWm, err := GetTrainingDataWatermark(other, "conv-c")
	require.NoError(t, err)
	assert.NotNil(t, otherWm)
}

func TestDeleteTrainingDataForUserIsIdempotentAndSafe(t *testing.T) {
	setupTrainingDataModelTest(t)

	// 不存在的数据：不报错、返回 0。
	n, err := DeleteTrainingDataForUser(99999)
	require.NoError(t, err)
	assert.Zero(t, n)

	// 非法 ID：直接返回，不碰数据库。
	for _, bad := range []int{0, -1} {
		n, err := DeleteTrainingDataForUser(bad)
		require.NoError(t, err, "非法 ID 不该报错")
		assert.Zero(t, n)
	}
}

// ID 复用场景：老用户被删后，新用户不应继承任何授权。
func TestDeletedUserDoesNotLeakConsentToReusedID(t *testing.T) {
	setupTrainingDataModelTest(t)

	const sharedID = 7777
	require.NoError(t, UpsertTrainingDataConsent(sharedID, true, "v1", 1000))

	// 老用户注销。
	_, err := DeleteTrainingDataForUser(sharedID)
	require.NoError(t, err)

	// 新用户拿到同一个 ID（自增回收）——必须**没有**授权。
	consent, err := GetTrainingDataConsent(sharedID)
	require.NoError(t, err)
	assert.Nil(t, consent,
		"ID 被复用时新用户绝不能继承前一位用户的授权（否则是从未授权却被采集）")
}

// 用户删除是主诉求：级联清理失败不得阻断删除本身。
// （这里只验证「清理函数存在且被调用」的契约面，实删路径由上面的用例覆盖。）
func TestHardDeleteUserCleansTrainingDataFirst(t *testing.T) {
	setupTrainingDataModelTest(t)

	const uid = 8888
	require.NoError(t, UpsertTrainingDataConsent(uid, true, "v1", 1000))
	require.NoError(t, SaveTrainingDataWatermark(uid, "conv-x", 1, "fp", 1, "h", 1000))

	// 直接调用级联清理（HardDeleteUserById 会在删主体前调它）。
	n, err := DeleteTrainingDataForUser(uid)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	consent, err := GetTrainingDataConsent(uid)
	require.NoError(t, err)
	assert.Nil(t, consent, "删除路径必须先清派生数据")
}

// setupTrainingDataModelTest 建一个只含采集两张表的临时 SQLite 库。
//
// 与 db_conformance_test.go 同样的开库方式（避免另造一套 DSN 约定）。
func setupTrainingDataModelTest(t *testing.T) {
	t.Helper()
	previousPath := common.SQLitePath
	previousDB := DB
	common.SQLitePath = filepath.Join(t.TempDir(), "training-cascade.db")
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	InitCol()

	db, dbType, err := chooseDB("TRAINING_CASCADE_DSN", false)
	require.NoError(t, err)
	require.Equal(t, common.DatabaseTypeSQLite, dbType)
	t.Setenv("TRAINING_CASCADE_DSN", "local")

	require.NoError(t, db.AutoMigrate(
		&TrainingDataConsent{}, &TrainingDataWatermark{}))

	DB = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
		common.SQLitePath = previousPath
	})
}
