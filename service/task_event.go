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
// task_event.go 提供任务生命周期事件的统一记录入口（B4-1 任务事件流 SSE）。
//
// RecordTaskEvent 是唯一写入点，best-effort：事件落库失败只记 SysError，
// 绝不阻塞任务主流程（状态转换/计费照常）。所有事件带 ts 与结构化 data。
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// 事件类型（B4-1 契约，前端按此文案/图标映射）。
const (
	TaskEventSubmitted   = "submitted"
	TaskEventQueued      = "queued"
	TaskEventClaimed     = "claimed"
	TaskEventProgress    = "progress"
	TaskEventStepRatio   = "step_ratio"
	TaskEventSucceeded   = "succeeded"
	TaskEventFailed      = "failed"
	TaskEventRefunded    = "refunded"
	TaskEventUnconfirmed = "unconfirmed"
)

// RecordTaskEvent 追加一条任务事件（best-effort）。
// taskID 为任务公开/第三方 id（用于展示与排查），internalID 为 Task.ID 主键。
func RecordTaskEvent(ctx context.Context, internalID int64, taskID string, userID int, eventType string, data any) {
	raw := []byte("{}")
	if data != nil {
		if b, err := common.Marshal(data); err == nil {
			raw = b
		}
	}
	ev := &model.TaskEvent{
		InternalTaskID: internalID,
		TaskID:         taskID,
		UserId:         userID,
		Type:           eventType,
		CreatedAt:      time.Now().Unix(),
		Data:           raw,
	}
	if _, err := model.InsertTaskEventWithContext(ctx, ev); err != nil {
		common.SysError(fmt.Sprintf("record task event error task=%d(%s) type=%s: %s", internalID, taskID, eventType, err.Error()))
	}
}

// TaskEventPayloadSubmitted 提交成功事件数据。
type TaskEventPayloadSubmitted struct {
	TaskID   string `json:"task_id"`
	Platform string `json:"platform"`
	Action   string `json:"action,omitempty"`
}

// TaskEventPayloadStatus 普通状态事件数据（queued/succeeded/failed 等）。
type TaskEventPayloadStatus struct {
	TaskID   string `json:"task_id"`
	Status   string `json:"status,omitempty"`
	Progress string `json:"progress,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Quota    int    `json:"quota,omitempty"`
	URL      string `json:"url,omitempty"`
}

// TaskEventPayloadProgress 进度事件数据（progress/step_ratio）。
type TaskEventPayloadProgress struct {
	TaskID    string `json:"task_id"`
	Progress  string `json:"progress,omitempty"`
	EventType string `json:"event_type,omitempty"`
	Current   int    `json:"current,omitempty"`
	Total     int    `json:"total,omitempty"`
	Step      string `json:"step,omitempty"`
}

// TaskEventPayloadUnconfirmed unconfirmed 事件数据。
type TaskEventPayloadUnconfirmed struct {
	TaskID           string `json:"task_id"`
	RemoteTaskIDHint string `json:"remote_task_id_hint,omitempty"`
	FailedAt         int64  `json:"failed_at,omitempty"`
}

// TaskEventRefunded 退款事件数据。
type TaskEventPayloadRefunded struct {
	TaskID string `json:"task_id"`
	Quota  int    `json:"quota"`
	Reason string `json:"reason,omitempty"`
}
