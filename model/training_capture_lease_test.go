package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTrainingCaptureLeaseExclusiveAndExpires(t *testing.T) {
	previous := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&SystemTaskLock{}))
	DB = db
	t.Cleanup(func() { DB = previous })

	ok, err := AcquireTrainingCaptureLease(42, "instance-a", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = AcquireTrainingCaptureLease(42, "instance-b", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "租约有效期内其他实例不得获取")

	require.NoError(t, ReleaseTrainingCaptureLease(42, "instance-a"))
	ok, err = AcquireTrainingCaptureLease(42, "instance-b", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "释放后其他实例可获取")

	// 过期后允许抢占。
	require.NoError(t, DB.Model(&SystemTaskLock{}).Where("type = ?", trainingCaptureLeaseType(42)).
		Update("locked_until", 1).Error)
	ok, err = AcquireTrainingCaptureLease(42, "instance-c", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "过期租约可被抢占")
}
