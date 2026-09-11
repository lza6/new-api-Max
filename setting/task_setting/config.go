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
package task_setting

import "github.com/lza6/new-api-Max/setting/config"

// TaskSetting 异步任务全局运行参数（热更新，注册名为 "task"）。
type TaskSetting struct {
	// UnconfirmedWindowMinutes 任务提交结果不可确认（unconfirmed）
	// 时的兜底查询窗口（分钟）。超过该窗口仍无法确认远端任务状态，
	// 则按平台语义退款并标记 resolution=refunded_after_window。
	// 0 或负数表示禁用超窗退款（任务持续以 unconfirmed 状态挂起）。
	UnconfirmedWindowMinutes int `json:"unconfirmed_window_minutes"`
}

// DefaultUnconfirmedWindowMinutes 默认 30 分钟。
const DefaultUnconfirmedWindowMinutes = 30

var taskSetting = TaskSetting{
	UnconfirmedWindowMinutes: DefaultUnconfirmedWindowMinutes,
}

func init() {
	config.GlobalConfig.Register("task", &taskSetting)
}

func GetTaskSetting() *TaskSetting {
	return &taskSetting
}

// GetUnconfirmedWindowMinutes 返回 unconfirmed 兜底查询窗口（分钟）。
// 未通过数据库配置时使用默认 30 分钟。
func GetUnconfirmedWindowMinutes() int {
	if s := GetTaskSetting(); s != nil && s.UnconfirmedWindowMinutes > 0 {
		return s.UnconfirmedWindowMinutes
	}
	return DefaultUnconfirmedWindowMinutes
}
