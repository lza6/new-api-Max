package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
)

// notifyLimitStore is used for in-memory rate limiting when Redis is disabled
var (
	notifyLimitStore sync.Map
	cleanupOnce      sync.Once
)

type limitCount struct {
	Count     int
	Timestamp time.Time
}

// getDuration 返回限流窗口长度。
//
// 非法/未初始化（<=0）时回退到 10 分钟：否则「窗口长度 0」会让内存实现
// 每次都判定过期（计数永远回到 0，限流永久失效）。
func getDuration() time.Duration {
	minute := constant.NotificationLimitDurationMinute
	if minute <= 0 {
		minute = 10
	}
	return time.Duration(minute) * time.Minute
}

// startCleanupTask starts a background task to clean up expired entries
func startCleanupTask() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour)
			now := time.Now()
			notifyLimitStore.Range(func(key, value any) bool {
				if limit, ok := value.(limitCount); ok {
					if now.Sub(limit.Timestamp) >= getDuration() {
						notifyLimitStore.Delete(key)
					}
				}
				return true
			})
		}
	})
}

// CheckNotificationLimit checks if the user has exceeded their notification limit
// Returns true if the user can send notification, false if limit exceeded
func CheckNotificationLimit(userId int, notifyType string) (bool, error) {
	if common.RedisEnabled {
		return checkRedisLimit(userId, notifyType)
	}
	return checkMemoryLimit(userId, notifyType)
}

func checkRedisLimit(userId int, notifyType string) (bool, error) {
	// **键不能带小时后缀**：带 `2006010215` 会让窗口每小时重置一次，
	// 配置 >60 分钟的时长完全失效；且跨小时的两个请求落在不同键上，
	// 统计口径被切成两段。固定窗口由 TTL 表达（见 RedisIncrWithTTL）。
	key := fmt.Sprintf("notify_limit:%d:%s", userId, notifyType)

	count, err := common.RedisIncrWithTTL(key, 1, getDuration())
	if err != nil {
		return false, fmt.Errorf("failed to count notification: %w", err)
	}
	return count <= int64(constant.NotifyLimitCount), nil
}

func checkMemoryLimit(userId int, notifyType string) (bool, error) {
	// Ensure cleanup task is started
	cleanupOnce.Do(startCleanupTask)

	// 与 Redis 版一致：不带小时后缀，窗口长度由 getDuration 决定。
	key := fmt.Sprintf("%d:%s", userId, notifyType)
	now := time.Now()

	// Get current limit count or initialize new one
	var currentLimit limitCount
	if value, ok := notifyLimitStore.Load(key); ok {
		currentLimit = value.(limitCount)
		// Check if the entry has expired
		if now.Sub(currentLimit.Timestamp) >= getDuration() {
			currentLimit = limitCount{Count: 0, Timestamp: now}
		}
	} else {
		currentLimit = limitCount{Count: 0, Timestamp: now}
	}

	// Increment count
	currentLimit.Count++

	// Check against limits
	limit := constant.NotifyLimitCount

	// Store updated count
	notifyLimitStore.Store(key, currentLimit)

	return currentLimit.Count <= limit, nil
}
