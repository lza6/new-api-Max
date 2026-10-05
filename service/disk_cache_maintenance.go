/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package service

import (
	"context"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// diskCacheCleanupInterval 磁盘缓存维护周期。请求结束会删除自己的缓存文件，
// 因此正常稳态占用很低；本循环只回收崩溃/异常退出留下的残留文件，并保证缓存
// 总大小不越过配置上限。
const diskCacheCleanupInterval = 30 * time.Minute

// diskCacheStaleAge 残留文件的最长保留时间。超过此年龄、且不属于任何在途请求
// 的缓存文件视为残留并删除。
const diskCacheStaleAge = 30 * time.Minute

var diskCacheMaintenanceLoop backgroundLoop

// StartDiskCacheMaintenanceLoop 周期性维护磁盘缓存：先按容量上限淘汰最旧文件，
// 再清理超龄残留。仅主节点执行，多节点去重；幂等，失败仅告警。
func StartDiskCacheMaintenanceLoop() {
	if !common.IsMasterNode {
		return
	}
	diskCacheMaintenanceLoop.start(func(ctx context.Context) {
		// 启动时清理一次历史残留（与 main.go 的启动清理互为补充）。
		runDiskCacheMaintenance()
		ticker := time.NewTicker(diskCacheCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runDiskCacheMaintenance()
			}
		}
	})
}

// StopDiskCacheMaintenanceLoop stops the maintenance loop and waits for it to
// exit (called from the graceful-shutdown sequence).
func StopDiskCacheMaintenanceLoop() {
	diskCacheMaintenanceLoop.stop()
}

func runDiskCacheMaintenance() {
	if !common.IsDiskCacheEnabled() {
		return
	}
	// 先按容量上限淘汰（reserve 0 表示只把总量压回上限内）。
	if removed, err := common.EnforceDiskCacheCapacity(common.DiskCacheEvictionSafeWindow, 0); err != nil {
		common.SysError("disk cache capacity enforcement error: " + err.Error())
	} else if removed > 0 {
		common.SysLog("disk cache capacity enforcement removed stale files")
	}
	// 再清理超龄残留（崩溃留下的孤儿文件）。
	if err := common.CleanupOldDiskCacheFiles(diskCacheStaleAge); err != nil {
		common.SysError("disk cache cleanup error: " + err.Error())
	}
}
