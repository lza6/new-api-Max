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
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// webProtectionCleanupInterval 自动清理周期：过期封禁与超期日志（保留 7 天）。
const webProtectionCleanupInterval = time.Hour

// StartWebProtectionMaintenanceLoop 定时执行 Web 防护维护（B1-3）。
// 复用 RunWebProtectionMaintenance（flush 未落库日志 + 清理过期封禁 + 超期日志）。
// 仅主节点执行，多节点去重；幂等，失败仅告警。
func StartWebProtectionMaintenanceLoop() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		_ = RunWebProtectionMaintenance()
		ticker := time.NewTicker(webProtectionCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := RunWebProtectionMaintenance(); err != nil {
				common.SysError("web protection maintenance error: " + err.Error())
			}
			// 周期清理渠道轮询锁缓存，防止 long-run 增删渠道时内存无限增长。
			model.CleanupChannelPollingLocks()
		}
	}()
}
