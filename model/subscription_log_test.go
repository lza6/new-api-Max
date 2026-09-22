package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetAllSubscriptionLogsJoinsUserAndPlan(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&UserSubscription{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&SubscriptionPlan{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&UserSubscription{}).Error)
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&SubscriptionPlan{}).Error)
	})

	user := User{Username: "sublog-user", DisplayName: "sublog-user", Role: 1, Status: 1}
	require.NoError(t, DB.Create(&user).Error)
	plan := SubscriptionPlan{Title: "Day Card", PriceAmount: 2, Currency: "CNY", DurationUnit: "day", DurationValue: 1}
	require.NoError(t, DB.Create(&plan).Error)
	sub := UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		Source:      "balance",
		Status:      "active",
		StartTime:   1000,
		EndTime:     1000 + 86400,
		AmountTotal: 1,
		AmountUsed:  0,
	}
	require.NoError(t, DB.Create(&sub).Error)

	result, err := GetAllSubscriptionLogs(1, 20, SubscriptionLogFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, result.Items, 1)
	item := result.Items[0]
	require.Equal(t, "sublog-user", item.Username)
	require.Equal(t, "Day Card", item.PlanTitle)
	require.Equal(t, float64(2), item.PriceAmount)
	require.Equal(t, "CNY", item.Currency)
	require.Equal(t, "balance", item.Source)
	require.Equal(t, "active", item.Status)

	filtered, err := GetAllSubscriptionLogs(1, 20, SubscriptionLogFilter{Username: "nope"})
	require.NoError(t, err)
	require.Equal(t, int64(0), filtered.Total)

	byStatus, err := GetAllSubscriptionLogs(1, 20, SubscriptionLogFilter{Status: "expired"})
	require.NoError(t, err)
	require.Equal(t, int64(0), byStatus.Total)

	bySource, err := GetAllSubscriptionLogs(1, 20, SubscriptionLogFilter{Source: "redemption"})
	require.NoError(t, err)
	require.Equal(t, int64(0), bySource.Total)
}
