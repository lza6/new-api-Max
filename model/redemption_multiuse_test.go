package model

import (
	"testing"

	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRedemptionTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Redemption{}, &RedemptionUsage{}, &User{}))
	original := DB
	DB = db
	t.Cleanup(func() { DB = original })
}

func seedRedemptionUser(t *testing.T, id int) {
	t.Helper()
	var existing User
	if err := DB.Where("id = ?", id).First(&existing).Error; err == nil {
		return
	}
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: fmt.Sprintf("u%d", id),
		Status:   1,
		AffCode:  common.GetRandomString(8), // 唯一邀请码
	}).Error)
}

func TestRedeemOneTimeCode(t *testing.T) {
	setupRedemptionTest(t)
	seedRedemptionUser(t, 1)

	code := &Redemption{Key: "once1", Quota: 100, Status: 1, MaxUses: 0}
	require.NoError(t, DB.Create(code).Error)

	got, err := Redeem("once1", 1)
	require.NoError(t, err)
	assert.Equal(t, 100, got.Quota)

	// 第二次兑换：一次性码已被使用。
	_, err = Redeem("once1", 1)
	assert.Error(t, err)
	assert.Equal(t, ErrRedeemFailed, err)
}

func TestRedeemMultiUseCode(t *testing.T) {
	setupRedemptionTest(t)
	seedRedemptionUser(t, 2)

	code := &Redemption{Key: "multi3", Quota: 50, Status: 1, MaxUses: 3}
	require.NoError(t, DB.Create(code).Error)

	// 三个不同用户各兑一次成功。
	for _, uid := range []int{2, 3, 4} {
		seedRedemptionUser(t, uid)
		got, err := Redeem("multi3", uid)
		require.NoError(t, err)
		assert.Equal(t, 50, got.Quota)
	}

	// 剩余归零后不可再兑（且状态被标为 disabled）。
	seedRedemptionUser(t, 5)
	_, err := Redeem("multi3", 5)
	assert.Error(t, err)

	var reloaded Redemption
	require.NoError(t, DB.First(&reloaded, code.Id).Error)
	assert.Equal(t, 2, reloaded.Status, "归零后状态应为 disabled")
}

// TestRedeemMultiUseConcurrentSameRemaining 并发兑换多用户码时不会超卖。
func TestRedeemMultiUseConcurrentSameRemaining(t *testing.T) {
	setupRedemptionTest(t)
	for _, uid := range []int{10, 11, 12, 13, 14} {
		seedRedemptionUser(t, uid)
	}
	code := &Redemption{Key: "conc2", Quota: 10, Status: 1, MaxUses: 2}
	require.NoError(t, DB.Create(code).Error)

	results := make(chan error, 5)
	for _, uid := range []int{10, 11, 12, 13, 14} {
		go func(uid int) { _, err := Redeem("conc2", uid); results <- err }(uid)
	}
	success := 0
	for range 5 {
		if err := <-results; err == nil {
			success++
		}
	}
	assert.Equal(t, 2, success, "MaxUses=2 时并发最多成功 2 次")
}

func TestRedeemMultiUseCodePerUserOnce(t *testing.T) {
	setupRedemptionTest(t)
	seedRedemptionUser(t, 1)
	seedRedemptionUser(t, 2)

	code := &Redemption{Key: "multi-peruser", Quota: 100, Status: 1, MaxUses: 2}
	require.NoError(t, DB.Create(code).Error)

	// 用户 1 首次兑换成功
	_, err := Redeem("multi-peruser", 1)
	require.NoError(t, err)

	// 用户 1 再次兑换同一码：每用户限一次，应报错
	_, err = Redeem("multi-peruser", 1)
	require.Error(t, err)

	// 用户 2 可正常兑换（次数上限 2）
	_, err = Redeem("multi-peruser", 2)
	require.NoError(t, err)

	// 次数耗尽后第三位用户被拒
	seedRedemptionUser(t, 3)
	_, err = Redeem("multi-peruser", 3)
	require.Error(t, err)
}
