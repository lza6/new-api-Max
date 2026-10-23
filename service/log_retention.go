package service

import (
	"strconv"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting"
)

const (
	// 检查间隔。删除按「保留天数」滚动计算目标时间戳，重复触发无副作用，
	// 所以用 6 小时而非严格每天——错过一次也不会漏删。
	logRetentionCheckInterval = 6 * time.Hour
)

var (
	logRetentionStartOnce sync.Once
	logRetentionStopOnce  sync.Once
	logRetentionStopCh    chan struct{}
)

// StartLogRetentionTask 按 setting.LogRetentionDays 定期删除过期消费日志。
//
// 只在 master 上运行：清理本身幂等，但多副本会各自入队 log_cleanup 任务，
// 互相抢同一批行（system_task 的活跃任务去重只在本进程内生效）。
//
// 复用既有的 log_cleanup 系统任务（分批删除 + 进度上报 + 可中断），
// 而不是一次性 DELETE 整表——184 万行的单事务删除会把 WAL 撑爆并长时间锁表。
func StartLogRetentionTask() {
	logRetentionStartOnce.Do(func() {
		logRetentionStopCh = make(chan struct{})
		go func() {
			// 启动先跑一次，避免部署后要等 6 小时才生效。
			runLogRetentionOnce()
			ticker := time.NewTicker(logRetentionCheckInterval)
			defer ticker.Stop()
			for {
				select {
				case <-logRetentionStopCh:
					return
				case <-ticker.C:
					runLogRetentionOnce()
				}
			}
		}()
	})
}

// StopLogRetentionTask 停机时结束保留策略循环（已入队的分批删除任务由 runner 继续消化）。
func StopLogRetentionTask() {
	logRetentionStopOnce.Do(func() {
		if logRetentionStopCh != nil {
			close(logRetentionStopCh)
		}
	})
}

func runLogRetentionOnce() {
	days := setting.LogRetentionDays
	if days <= 0 {
		return
	}
	target := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	if _, err := StartLogCleanupTask(target); err != nil {
		common.SysError("log retention: failed to enqueue cleanup task: " + err.Error())
		return
	}
	common.SysLog("log retention: cleanup enqueued (retain " + strconv.Itoa(days) + " days)")
}
