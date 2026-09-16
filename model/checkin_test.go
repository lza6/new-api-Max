package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCheckinDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Checkin{}))
	DB = db
	t.Cleanup(func() { DB = prevDB })
	return db
}

func TestResetAllCheckins(t *testing.T) {
	setupCheckinDB(t)
	require.NoError(t, DB.Create(&Checkin{UserId: 1, CheckinDate: "2026-09-16", QuotaAwarded: 100}).Error)
	require.NoError(t, DB.Create(&Checkin{UserId: 2, CheckinDate: "2026-09-16", QuotaAwarded: 200}).Error)

	deleted, err := ResetAllCheckins()
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	var count int64
	require.NoError(t, DB.Model(&Checkin{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestCheckinFixedQuotaWhenMinEqualsMax(t *testing.T) {
	setupCheckinDB(t)
	prev := operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{
		Enabled:  true,
		MinQuota: 50000,
		MaxQuota: 50000, // min=max → 固定额度
	}
	common.RedisEnabled = false
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = *prev })

	require.NoError(t, DB.Create(&User{Id: 9, Username: "checkin-u", Role: 1, Group: "default"}).Error)
	checkin, err := UserCheckin(9)
	require.NoError(t, err)
	assert.Equal(t, 50000, checkin.QuotaAwarded, "min==max 时应固定发 min")

	// 同日重复签到被拒
	_, err = UserCheckin(9)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已签到")

	// 重置后同一用户可再次签到
	n, err := ResetAllCheckins()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	checkin2, err := UserCheckin(9)
	require.NoError(t, err)
	assert.Equal(t, 50000, checkin2.QuotaAwarded)
}

func TestHasCheckedInTodayUsesServerDate(t *testing.T) {
	setupCheckinDB(t)
	today := time.Now().Format("2006-01-02")
	require.NoError(t, DB.Create(&Checkin{UserId: 3, CheckinDate: today, QuotaAwarded: 1}).Error)
	ok, err := HasCheckedInToday(3)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = HasCheckedInToday(4)
	require.NoError(t, err)
	assert.False(t, ok)
}
