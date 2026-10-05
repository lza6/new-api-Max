package service

import (
	"context"
	"strconv"
	"time"

	"github.com/lza6/new-api-Max/common"
)

const taskArtifactCleanupInterval = 5 * time.Minute

var taskArtifactCleanupLoop backgroundLoop

// StartTaskArtifactCleanup 启动站内图床清理后台 loop：每 5 分钟清理「超过保留期的
// 已完成任务产物/参考素材」。仅在本地图床启用时生效（否则 no-op）。
// 仅主节点执行（多节点各自清理本地磁盘，但为避免重复日志由 master 记录）。
func StartTaskArtifactCleanup() {
	if LocalArtifactStore() == nil {
		return
	}
	taskArtifactCleanupLoop.start(func(ctx context.Context) {
		ticker := time.NewTicker(taskArtifactCleanupInterval)
		defer ticker.Stop()
		runTaskArtifactCleanupOnce()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runTaskArtifactCleanupOnce()
			}
		}
	})
}

// StopTaskArtifactCleanup 停止清理 loop（供优雅关闭/测试调用）。
func StopTaskArtifactCleanup() {
	taskArtifactCleanupLoop.stop()
}

func runTaskArtifactCleanupOnce() {
	store := LocalArtifactStore()
	if store == nil {
		return
	}
	retainBefore := time.Now().Add(-artifactRetentionFromEnv())
	removed, err := store.CleanupExpiredArtifacts(retainBefore)
	if err != nil {
		common.SysError("task artifact cleanup failed: " + err.Error())
		return
	}
	if removed > 0 {
		common.SysLog("task artifact cleanup: removed " + strconv.Itoa(removed) + " expired task artifact dirs")
	}
}
