package service

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 通知限流的窗口必须是**固定窗口**，不能按小时切段：
// 旧实现把小时拼进 key，`NOTIFICATION_LIMIT_DURATION_MINUTE > 60` 完全失效，
// 且跨小时的请求会落到不同 key，统计被切成两段。
func TestNotifyLimitFixedWindowMemory(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })

	prevCount := constant.NotifyLimitCount
	constant.NotifyLimitCount = 2
	t.Cleanup(func() { constant.NotifyLimitCount = prevCount })

	prevDuration := constant.NotificationLimitDurationMinute
	constant.NotificationLimitDurationMinute = 120
	t.Cleanup(func() { constant.NotificationLimitDurationMinute = prevDuration })

	const userID = 880001
	const notifyType = "unit-test"
	// 同一进程重复运行（-count=2）会复用 sync.Map；先清掉上次遗留的计数。
	notifyLimitStore.Delete("880001:unit-test")
	t.Cleanup(func() { notifyLimitStore.Delete("880001:unit-test") })
	for i := range 2 {
		ok, err := CheckNotificationLimit(userID, notifyType)
		require.NoError(t, err)
		assert.True(t, ok, "第 %d 次应允许", i+1)
	}
	ok, err := CheckNotificationLimit(userID, notifyType)
	require.NoError(t, err)
	assert.False(t, ok, "超过上限必须拒绝")
}

// 非法的窗口长度（<=0）必须回退到默认值，否则内存实现每次都判定过期，
// 计数永远归零 → 限流永久失效。
func TestNotifyLimitDurationFallsBack(t *testing.T) {
	prev := constant.NotificationLimitDurationMinute
	constant.NotificationLimitDurationMinute = 0
	t.Cleanup(func() { constant.NotificationLimitDurationMinute = prev })

	assert.Equal(t, 10*time.Minute, getDuration())
}
