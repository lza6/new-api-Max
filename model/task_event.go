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
// TaskEvent 记录任务生命周期事件（B4-1 任务事件流 SSE）。
//
// ID 是数据库自增主键，全局单调递增，作为事件 seq：断线续传用
// `id > sinceSeq` 拉取，天然无重复；事务回滚产生的空洞不影响正确性
// （后续事件 id 仍严格递增，客户端按 id 排序无遗漏）。
// InternalTaskID 关联 Task.ID（主键，稳定跟踪 key——轮询期间 Task.TaskID
// 可能被替换为上游 id）。读路径仅查本表，事件写入是 best-effort，
// 失败不影响任务主流程。
package model

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type TaskEvent struct {
	ID             int64           `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	InternalTaskID int64           `json:"internal_task_id" gorm:"index"`
	TaskID         string          `json:"task_id" gorm:"type:varchar(191);index"` // 公开/第三方任务 id，便于排查
	UserId         int             `json:"user_id" gorm:"index"`
	Type           string          `json:"type" gorm:"type:varchar(30)"`
	CreatedAt      int64           `json:"created_at"`
	Data           json.RawMessage `json:"data" gorm:"type:json"`
}

// InsertTaskEvent 追加一条任务事件；返回自增 seq。
func InsertTaskEvent(ev *TaskEvent) (int64, error) {
	if ev.CreatedAt == 0 {
		ev.CreatedAt = time.Now().Unix()
	}
	if len(ev.Data) == 0 {
		ev.Data = json.RawMessage("{}")
	}
	err := DB.Create(ev).Error
	return ev.ID, err
}

// InsertTaskEventWithContext 支持携带调用方 context 的追加。
func InsertTaskEventWithContext(ctx context.Context, ev *TaskEvent) (int64, error) {
	if ev.CreatedAt == 0 {
		ev.CreatedAt = time.Now().Unix()
	}
	if len(ev.Data) == 0 {
		ev.Data = json.RawMessage("{}")
	}
	err := DB.WithContext(ctx).Create(ev).Error
	return ev.ID, err
}

// ListTaskEventsAfter 返回 internalTaskID 下 seq 大于 sinceSeq 的事件（升序）。
func ListTaskEventsAfter(internalTaskID int64, sinceSeq int64, limit int) ([]*TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []*TaskEvent
	err := DB.Where("internal_task_id = ? AND id > ?", internalTaskID, sinceSeq).
		Order("id asc").Limit(limit).Find(&events).Error
	return events, err
}

// LastTaskEventSeq 返回某任务最后一条事件的 seq（无事件返回 0）。
func LastTaskEventSeq(internalTaskID int64) (int64, error) {
	var ev TaskEvent
	err := DB.Where("internal_task_id = ?", internalTaskID).
		Order("id desc").Limit(1).First(&ev).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return ev.ID, nil
}
