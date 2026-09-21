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
// task_event.go 提供任务事件流 SSE 端点（B4-1）。
// GET /api/task/:task_id/events?since=<seq>（UserAuth）
// - 归属校验：按当前用户 + task_id 查任务，不存在/越权则 404。
// - 协议：text/event-stream；每条 `id:<seq>\nevent:<type>\ndata:<json>\n\n`；
//   空轮询发 `: ping` heartbeat；任务已终态且事件推送完毕发 `event: done` 关闭。
// - 断线续传：客户端带 since（query 或 Last-Event-ID）重连，服务端只回后续事件，
//   配合 DB 自增 id 语义保证 no gap/dup。
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
)

// ssePollInterval 事件轮询间隔。
const ssePollInterval = 1 * time.Second

// sseEventEnvelope SSE 事件信封：每条事件携带 seq（断线续传游标）与 ts，
// payload 为结构化业务数据（B4-1 契约：事件含 ts + 结构化 data）。
type sseEventEnvelope struct {
	Seq     int64           `json:"seq"`
	Ts      int64           `json:"ts"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// TaskEventsSSE 流式返回某任务的生命周期事件。
func TaskEventsSSE(c *gin.Context) {
	taskID := c.Param("task_id")
	userID := c.GetInt("id")
	task, exists, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		common.SysError(fmt.Sprintf("query task for sse error task=%s: %s", taskID, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query task"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	since := parseTaskEventSince(c)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	streamTaskEvents(c.Writer, c.Writer.Flush, task, c.Request.Context(), since)
}

// streamTaskEvents 是 SSE 推送核心：按 since 续传拉取事件写 w，空轮询发 heartbeat，
// 任务终态且事件推送完毕发 done 事件后返回。ctx 取消即停止。
// 与 gin 解耦便于单元测试（可传 bytes.Buffer + no-op flush + 可控 context）。
func streamTaskEvents(w io.Writer, flush func(), task *model.Task, ctx context.Context, since int64) {
	ticker := time.NewTicker(ssePollInterval)
	defer ticker.Stop()

	lastSent := since
	doneSent := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := model.ListTaskEventsAfter(task.ID, lastSent, 100)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("list task events error task=%d: %s", task.ID, err.Error()))
				continue
			}
			for _, ev := range events {
				envelope := sseEventEnvelope{Seq: ev.ID, Ts: ev.CreatedAt, Type: ev.Type, Payload: ev.Data}
				raw, marshalErr := common.Marshal(envelope)
				if marshalErr != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, string(raw)); err != nil {
					return
				}
				lastSent = ev.ID
			}
			if len(events) > 0 {
				flush()
			} else {
				// heartbeat，保持连接并穿透代理缓冲
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				flush()
			}

			// 任务进入终态且已推送完现有事件 → 发 done 并关闭流。
			// P1-3 修复：终态时必须确认没有滞后事件（单批 100 上限可能
			// 只拉了一部分），否则 done 提前关闭会截断丢事件。
			status := task.Status
			if current, _, err := model.GetByTaskId(task.UserId, task.TaskID); err == nil {
				status = current.Status
			}
			terminal := status == model.TaskStatusSuccess || status == model.TaskStatusFailure
			drained := true
			if terminal {
				if lastSeq, err := model.LastTaskEventSeq(task.ID); err == nil && lastSeq > lastSent {
					drained = false
				}
			}
			if terminal && drained && !doneSent {
				doneSent = true
				if _, err := fmt.Fprint(w, "event: done\ndata: {}\n\n"); err != nil {
					return
				}
				flush()
				return
			}
		}
	}
}

// parseTaskEventSince 解析断线续传起点：query `since` 优先，其次 Last-Event-ID 头。
func parseTaskEventSince(c *gin.Context) int64 {
	if raw := c.Query("since"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	if raw := c.GetHeader("Last-Event-ID"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
