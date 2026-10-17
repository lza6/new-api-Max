package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedExpiryReminderDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, connErr := db.DB()
	require.NoError(t, connErr)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.User{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	return db
}

func createExpiryReminderSub(t *testing.T, db *gorm.DB, id, userId int, endTime int64, status string, reminded int) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:                   id,
		UserId:               userId,
		Status:               status,
		StartTime:            common.GetTimestamp(),
		EndTime:              endTime,
		ReminderDaysNotified: reminded,
	}
	require.NoError(t, db.Create(sub).Error)
}

// 命中窗口：end_time 在 (now, now+N*86400] 内且 reminder_days_notified < N 的 active 订阅。
func TestGetDueExpiryReminderSubscriptionsWindowHitAndMiss(t *testing.T) {
	db := seedExpiryReminderDB(t)
	now := common.GetTimestamp()
	const notifyDays = 3

	createExpiryReminderSub(t, db, 8001, 81001, now+2*86400, "active", 0)      // 窗口内 → 命中
	createExpiryReminderSub(t, db, 8002, 81002, now+3*86400-3600, "active", 0) // 窗口边界内 → 命中
	createExpiryReminderSub(t, db, 8003, 81003, now+3*86400+3600, "active", 0) // 超窗 → 不命中
	createExpiryReminderSub(t, db, 8004, 81004, now+86400, "active", 3)        // 已提醒 → 去重不命中
	createExpiryReminderSub(t, db, 8005, 81005, now-1, "active", 0)            // 已到期 → 不命中
	createExpiryReminderSub(t, db, 8006, 81006, now+86400, "cancelled", 0)     // 非 active → 不命中

	subs, err := model.GetDueExpiryReminderSubscriptions(100, notifyDays)
	require.NoError(t, err)
	ids := make([]int, 0, len(subs))
	for i := range subs {
		ids = append(ids, subs[i].Id)
	}
	assert.Equal(t, []int{8001, 8002}, ids)
}

// 去重：MarkSubscriptionReminderNotified 置位后同窗口不再命中，重复置位幂等。
func TestMarkSubscriptionReminderNotifiedIdempotentAndDedup(t *testing.T) {
	db := seedExpiryReminderDB(t)
	now := common.GetTimestamp()
	createExpiryReminderSub(t, db, 8101, 82001, now+86400, "active", 0)
	createExpiryReminderSub(t, db, 8102, 82002, now+86400, "active", 0)

	require.NoError(t, model.MarkSubscriptionReminderNotified(8101, 3))
	require.NoError(t, model.MarkSubscriptionReminderNotified(8101, 3)) // 幂等，不报错
	require.NoError(t, model.MarkSubscriptionReminderNotified(8102, 3))

	var sub model.UserSubscription
	require.NoError(t, db.Where("id = ?", 8101).First(&sub).Error)
	assert.Equal(t, 3, sub.ReminderDaysNotified)

	subs, err := model.GetDueExpiryReminderSubscriptions(100, 3)
	require.NoError(t, err)
	assert.Empty(t, subs) // 全部已提醒 → 去重生效
}

// 批处理：查询按 limit 截断；分批扫描靠每批置位推进（与 scan 循环行为一致），
// 已处理的上限满足后不再命中，逐批可拉完整集合。
func TestGetDueExpiryReminderSubscriptionsBatchLimit(t *testing.T) {
	db := seedExpiryReminderDB(t)
	now := common.GetTimestamp()
	for i := 0; i < 5; i++ {
		createExpiryReminderSub(t, db, 8200+i, 83000+i, now+2*86400, "active", 0)
	}

	first, err := model.GetDueExpiryReminderSubscriptions(2, 3)
	require.NoError(t, err)
	require.Len(t, first, 2)
	for i := range first {
		require.NoError(t, model.MarkSubscriptionReminderNotified(first[i].Id, 3))
	}

	second, err := model.GetDueExpiryReminderSubscriptions(2, 3)
	require.NoError(t, err)
	require.Len(t, second, 2)
	for i := range second {
		require.NoError(t, model.MarkSubscriptionReminderNotified(second[i].Id, 3))
	}

	third, err := model.GetDueExpiryReminderSubscriptions(2, 3)
	require.NoError(t, err)
	require.Len(t, third, 1)
}

// 端到端：扫描任务对命中订阅发送一次并置位，重复扫描不再命中（防每 tick 重复发）。
// 用户无 email/渠道时 NotifyUser 走成功路径并置位，避免测试依赖真实邮件发送。
func TestScanSubscriptionExpiryReminderSendsOnceAndDedups(t *testing.T) {
	db := seedExpiryReminderDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.User{Id: 84001, Username: "remind-user", Status: common.UserStatusEnabled}).Error)
	createExpiryReminderSub(t, db, 8401, 84001, now+86400, "active", 0)

	// 测试进程未初始化 Redis：显式走内存限频，保证 NotifyUser 限频路径确定性。
	// initConstantEnv（含 NotifyLimitCount 默认值）不在测试链路执行，需显式设置。
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prevNotifyLimit := constant.NotifyLimitCount
	constant.NotifyLimitCount = 2
	t.Cleanup(func() { constant.NotifyLimitCount = prevNotifyLimit })

	t.Setenv("SUBSCRIPTION_EXPIRY_REMIND_DAYS", "3")
	scanSubscriptionExpiryReminder()

	var sub model.UserSubscription
	require.NoError(t, db.Where("id = ?", 8401).First(&sub).Error)
	assert.Equal(t, 3, sub.ReminderDaysNotified) // 已置位 → 防重复

	subs, err := model.GetDueExpiryReminderSubscriptions(100, 3)
	require.NoError(t, err)
	assert.Empty(t, subs) // 再次扫描不再命中
}

// 多阈值：默认 3 天/1 天两次提醒，第二次不得被第一次的去重条件吞掉。
func TestParseSubscriptionExpiryReminderDays(t *testing.T) {
	assert.Equal(t, []int{3, 1}, parseSubscriptionExpiryReminderDays("3,1"))
	assert.Equal(t, []int{7, 3, 1}, parseSubscriptionExpiryReminderDays("1, 3,7,3,0,-1,x"))
	assert.Empty(t, parseSubscriptionExpiryReminderDays(""))
}

func TestGetDueExpiryReminderSubscriptionsBetweenWindows(t *testing.T) {
	db := seedExpiryReminderDB(t)
	now := common.GetTimestamp()

	createExpiryReminderSub(t, db, 8501, 85001, now+2*86400, "active", 0) // 落在 3 天窗口
	createExpiryReminderSub(t, db, 8502, 85002, now+12*3600, "active", 0) // 不足 1 天
	createExpiryReminderSub(t, db, 8503, 85003, now+12*3600, "active", 3) // 已收 3 天提醒，应进 1 天窗口

	threeDay, err := model.GetDueExpiryReminderSubscriptionsBetween(100, 1, 3)
	require.NoError(t, err)
	ids := make([]int, 0, len(threeDay))
	for i := range threeDay {
		ids = append(ids, threeDay[i].Id)
	}
	assert.Equal(t, []int{8501}, ids, "3 天窗口只应覆盖 (>1d, <=3d]，不得抢先打扰不足 1 天的订阅")

	oneDay, err := model.GetDueExpiryReminderSubscriptionsBetween(100, 0, 1)
	require.NoError(t, err)
	ids = ids[:0]
	for i := range oneDay {
		ids = append(ids, oneDay[i].Id)
	}
	assert.ElementsMatch(t, []int{8502, 8503}, ids, "1 天窗口应覆盖未提醒与已收 3 天提醒的订阅")
}

func TestScanSubscriptionExpiryReminderSendsTwoThresholds(t *testing.T) {
	db := seedExpiryReminderDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.User{Id: 86001, Username: "remind-two", Status: common.UserStatusEnabled}).Error)
	createExpiryReminderSub(t, db, 8601, 86001, now+2*86400, "active", 0)

	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prevNotifyLimit := constant.NotifyLimitCount
	constant.NotifyLimitCount = 5
	t.Cleanup(func() { constant.NotifyLimitCount = prevNotifyLimit })

	t.Setenv("SUBSCRIPTION_EXPIRY_REMIND_DAYS", "3,1")

	scanSubscriptionExpiryReminder()
	var sub model.UserSubscription
	require.NoError(t, db.Where("id = ?", 8601).First(&sub).Error)
	assert.Equal(t, 3, sub.ReminderDaysNotified, "先进入 3 天窗口")

	// 模拟时间推进到 12 小时后：同一条订阅应再收到一次 1 天提醒。
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", 8601).
		Update("end_time", now+12*3600).Error)
	scanSubscriptionExpiryReminder()
	require.NoError(t, db.Where("id = ?", 8601).First(&sub).Error)
	assert.Equal(t, 1, sub.ReminderDaysNotified, "进入 1 天窗口后必须再提醒一次")

	// 第三次扫描不得重复发送。
	scanSubscriptionExpiryReminder()
	require.NoError(t, db.Where("id = ?", 8601).First(&sub).Error)
	assert.Equal(t, 1, sub.ReminderDaysNotified)
}
