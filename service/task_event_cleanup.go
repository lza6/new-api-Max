package service

import (
	"context"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// taskEventCleanupInterval 任务事件清理周期。
const taskEventCleanupInterval = time.Hour

// taskEventRetentionDays 任务事件保留天数（env TASK_EVENT_RETENTION_DAYS，默认 7；
// 0 或负数表示不清理，避免破坏需要长期事件流的部署）。
var taskEventRetentionDays = common.GetEnvOrDefault("TASK_EVENT_RETENTION_DAYS", 7)

// StartTaskEventCleanup 周期清理过期任务事件，仅主节点执行（多实例防重复）。
// 与 B4-1 SSE 断线续传兼容：续传只依赖最近事件，保留窗口内的事件足够。
func StartTaskEventCleanup() {
	if !common.IsMasterNode || taskEventRetentionDays <= 0 {
		return
	}
	go func() {
		cleanupOldTaskEvents()
		ticker := time.NewTicker(taskEventCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cleanupOldTaskEvents()
		}
	}()
}

func cleanupOldTaskEvents() {
	before := time.Now().Add(-time.Duration(taskEventRetentionDays) * 24 * time.Hour).Unix()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		deleted, err := model.DeleteOldTaskEventsBatch(ctx, before, 1000)
		if err != nil {
			common.SysError("task event cleanup failed: " + err.Error())
			return
		}
		if deleted == 0 {
			return
		}
	}
}
