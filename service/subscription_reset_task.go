package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relaykit/dto"
)

const (
	subscriptionResetTickInterval = 1 * time.Minute
	subscriptionResetBatchSize    = 300
	subscriptionCleanupInterval   = 30 * time.Minute
)

var (
	subscriptionResetRunning atomic.Bool
	subscriptionCleanupLast  atomic.Int64
	// subscriptionResetTask 管理订阅过期/重置 loop 生命周期，供优雅关闭停止。
	subscriptionResetTask backgroundLoop
)

func StartSubscriptionQuotaResetTask() {
	if !common.IsMasterNode {
		return
	}
	subscriptionResetTask.start(func(ctx context.Context) {
		logger.LogInfo(ctx, fmt.Sprintf("subscription quota reset task started: tick=%s", subscriptionResetTickInterval))
		ticker := time.NewTicker(subscriptionResetTickInterval)
		defer ticker.Stop()

		runSubscriptionQuotaResetOnce()
		common.RecordLoopHeartbeat("subscription_quota_reset")
		for {
			select {
			case <-ctx.Done():
				logger.LogInfo(ctx, "subscription quota reset task stopped")
				return
			case <-ticker.C:
				runSubscriptionQuotaResetOnce()
				common.RecordLoopHeartbeat("subscription_quota_reset")
			}
		}
	})
}

// StopSubscriptionQuotaResetTask 停止订阅过期/重置后台任务并等待其退出（供优雅关闭调用）。
func StopSubscriptionQuotaResetTask() {
	subscriptionResetTask.stop()
}

func runSubscriptionQuotaResetOnce() {
	if !subscriptionResetRunning.CompareAndSwap(false, true) {
		return
	}
	defer subscriptionResetRunning.Store(false)

	ctx := context.Background()
	totalReset := 0
	totalExpired := 0
	for {
		n, err := model.ExpireDueSubscriptions(subscriptionResetBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("subscription expire task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		totalExpired += n
		if n < subscriptionResetBatchSize {
			break
		}
	}
	for {
		n, err := model.ResetDueSubscriptions(subscriptionResetBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("subscription quota reset task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		totalReset += n
		if n < subscriptionResetBatchSize {
			break
		}
	}
	// 到期前 N 天提醒：与过期/重置任务共用同一 MasterNode goroutine 与 CAS 锁。
	scanSubscriptionExpiryReminder()

	lastCleanup := time.Unix(subscriptionCleanupLast.Load(), 0)
	if time.Since(lastCleanup) >= subscriptionCleanupInterval {
		if _, err := model.CleanupSubscriptionPreConsumeRecords(7 * 24 * 3600); err == nil {
			subscriptionCleanupLast.Store(time.Now().Unix())
		}
	}
	if common.DebugEnabled && (totalReset > 0 || totalExpired > 0) {
		logger.LogDebug(ctx, "subscription maintenance: reset_count=%d, expired_count=%d", totalReset, totalExpired)
	}
}

// scanSubscriptionExpiryReminder 扫描到期前多个阈值（SUBSCRIPTION_EXPIRY_REMIND_DAYS，
// 逗号分隔，默认 "3,1"）的 active 订阅，对命中用户发送到期提醒（含续费链接），
// 并按阈值记录 ReminderDaysNotified，防止每个 tick 重复发送。
//
// 多阈值语义：3 天扫描只覆盖 (>1d, <=3d]；进入 1 天窗口后再发一次。
// 这样「3 天/1 天两次提醒」不会互相吞掉，也不会把不足 1 天的订阅先用 3 天文案打扰。
// 与过期/重置任务共用 runSubscriptionQuotaResetOnce 的 CAS 锁，仅 MasterNode 单跑。
func scanSubscriptionExpiryReminder() {
	days := parseSubscriptionExpiryReminderDays(
		common.GetEnvOrDefaultString("SUBSCRIPTION_EXPIRY_REMIND_DAYS", "3,1"),
	)
	if len(days) == 0 {
		return
	}
	ctx := context.Background()
	reminded := 0
	for i, notifyDays := range days {
		lowerDays := 0
		if i+1 < len(days) {
			lowerDays = days[i+1]
		}
		reminded += scanExpiryReminderWindow(ctx, lowerDays, notifyDays)
	}
	if common.DebugEnabled && reminded > 0 {
		logger.LogDebug(ctx, "subscription expiry reminder: notified=%d, thresholds=%v", reminded, days)
	}
}

// scanExpiryReminderWindow 处理一个阈值窗口，返回成功通知数。分批直到没有命中。
func scanExpiryReminderWindow(ctx context.Context, lowerDays, notifyDays int) int {
	reminded := 0
	for {
		subs, err := model.GetDueExpiryReminderSubscriptionsBetween(
			subscriptionResetBatchSize, lowerDays, notifyDays)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("subscription expiry reminder scan failed: %v", err))
			return reminded
		}
		if len(subs) == 0 {
			return reminded
		}
		for i := range subs {
			if err := notifySubscriptionExpiryReminder(&subs[i], notifyDays); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("subscription expiry reminder notify failed for sub %d: %v", subs[i].Id, err))
				continue
			}
			reminded++
		}
		if len(subs) < subscriptionResetBatchSize {
			return reminded
		}
	}
}

// parseSubscriptionExpiryReminderDays 解析逗号分隔的提醒阈值：
// 只保留正整数，去重并按从大到小排序（扫描顺序必须是 3 天 → 1 天）。
func parseSubscriptionExpiryReminderDays(raw string) []int {
	seen := make(map[int]struct{})
	days := make([]int, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		days = append(days, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(days)))
	return days
}

// notifySubscriptionExpiryReminder 对单个订阅发送到期提醒（邮件/渠道文案含到期时间与续费
// 链接），通知成功后才置位 ReminderDaysNotified 防止重复；发送失败留给下一 tick 重试。
// 通知限频由 NotifyUser 内部 CheckNotificationLimit 兜底。
func notifySubscriptionExpiryReminder(sub *model.UserSubscription, notifyDays int) error {
	if sub == nil || sub.UserId <= 0 {
		return nil
	}
	user, err := model.GetUserById(sub.UserId, false)
	if err != nil {
		return err
	}
	expireDate := time.Unix(sub.EndTime, 0).Format("2006-01-02 15:04")
	prompt := fmt.Sprintf("您的套餐将于 %s 到期", expireDate)
	renewLink := PaymentReturnURL("/subscriptions")
	content := "{{value}}，请及时续费保持服务不中断。<br/>续费链接：<a href='{{value}}'>{{value}}</a>"
	values := []any{prompt, renewLink, renewLink}
	if err := NotifyUser(sub.UserId, user.Email, user.GetSetting(),
		dto.NewNotify(dto.NotifyTypeSubscriptionExpiryReminder, prompt, content, values)); err != nil {
		return err
	}
	return model.MarkSubscriptionReminderNotified(sub.Id, notifyDays)
}
