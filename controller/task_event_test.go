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
package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/model"
)

func setupTaskEventSSEDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskEvent{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
	})
}

func seedTaskEvent(t *testing.T, taskID string, userID int, internalID int64, eventType string) {
	t.Helper()
	ev := &model.TaskEvent{
		InternalTaskID: internalID,
		TaskID:         taskID,
		UserId:         userID,
		Type:           eventType,
		CreatedAt:      time.Now().Unix(),
		Data:           []byte(`{"task_id":"` + taskID + `"}`),
	}
	_, err := model.InsertTaskEvent(ev)
	require.NoError(t, err)
}

// TestStreamTaskEventsFinalTaskEmitsDone：终态任务 → 事件行 + done，函数自动返回。
func TestStreamTaskEventsFinalTaskEmitsDone(t *testing.T) {
	setupTaskEventSSEDB(t)
	task := &model.Task{TaskID: "t-final", UserId: 1, Status: model.TaskStatusSuccess}
	require.NoError(t, task.Insert())
	seedTaskEvent(t, "t-final", 1, task.ID, "submitted")
	seedTaskEvent(t, "t-final", 1, task.ID, "succeeded")

	var buf bytes.Buffer
	streamTaskEvents(&buf, func() {}, task, context.Background(), 0)

	body := buf.String()
	require.Contains(t, body, "id: ")
	require.Contains(t, body, "data: ")
	require.Contains(t, body, "event: done")
	assert.Equal(t, 1, strings.Count(body, "event: done"), "done 只发一次")
	require.GreaterOrEqual(t, strings.Count(body, "id: "), 2)
}

// TestStreamTaskEventsSinceSeqResume：since=首条事件 id → 只回后续事件（无重复）。
func TestStreamTaskEventsSinceSeqResume(t *testing.T) {
	setupTaskEventSSEDB(t)
	task := &model.Task{TaskID: "t-resume", UserId: 1, Status: model.TaskStatusSuccess}
	require.NoError(t, task.Insert())
	seedTaskEvent(t, "t-resume", 1, task.ID, "submitted")
	seedTaskEvent(t, "t-resume", 1, task.ID, "queued")
	seedTaskEvent(t, "t-resume", 1, task.ID, "succeeded")

	first, err := model.ListTaskEventsAfter(task.ID, 0, 1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	firstID := first[0].ID

	var buf bytes.Buffer
	streamTaskEvents(&buf, func() {}, task, context.Background(), firstID)

	body := buf.String()
	require.NotContains(t, body, "id: "+strconv.FormatInt(firstID, 10), "since 之后不应重发已消费事件")
	require.Contains(t, body, "event: succeeded")
}

// TestStreamTaskEventsCancelStops：非终态任务 → ctx 取消即返回。
func TestStreamTaskEventsCancelStops(t *testing.T) {
	setupTaskEventSSEDB(t)
	task := &model.Task{TaskID: "t-active", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		defer close(done)
		streamTaskEvents(&buf, func() {}, task, ctx, 0)
	}()
	time.Sleep(1500 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("streamTaskEvents did not stop after context cancel")
	}
}

// TestStreamTaskEventsOtherUserDenied：越权访问他人任务的事件流必须被拒。
// GetByTaskId 按 user_id + task_id 定位，非属主查不到任务 → 404，不泄露
// 任何事件数据（验收标准：权限越权访问被拒）。
func TestStreamTaskEventsOtherUserDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTaskEventSSEDB(t)
	task := &model.Task{TaskID: "t-private", UserId: 1, Status: model.TaskStatusSuccess}
	require.NoError(t, task.Insert())
	seedTaskEvent(t, "t-private", 1, task.ID, "submitted")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/t-private/events", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "t-private"}}
	ctx.Set("id", 999) // 非属主
	ctx.Set("username", "intruder")

	TaskEventsSSE(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "submitted")
	require.NotContains(t, recorder.Body.String(), "event: ")
}

// TestStreamTaskEventsOwnerAllowed：属主本人可读事件流（成功路径闭环）。
func TestStreamTaskEventsOwnerAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTaskEventSSEDB(t)
	task := &model.Task{TaskID: "t-owner", UserId: 7, Status: model.TaskStatusSuccess}
	require.NoError(t, task.Insert())
	seedTaskEvent(t, "t-owner", 7, task.ID, "succeeded")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/t-owner/events?since=0", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "t-owner"}}
	ctx.Set("id", 7)

	TaskEventsSSE(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "event: done")
	require.Contains(t, recorder.Body.String(), "succeeded")
}

// TestStreamTaskEvents500EventsNoGapNoDup：500 条事件压测（验收标准）。
// 单批拉取上限 100，多轮 ticker 拉取必须把全部事件无缺无重复推完，终态
// 后发 done；body 中每个 id 恰好出现一次。
func TestStreamTaskEvents500EventsNoGapNoDup(t *testing.T) {
	setupTaskEventSSEDB(t)
	task := &model.Task{TaskID: "t-500", UserId: 1, Status: model.TaskStatusSuccess}
	require.NoError(t, task.Insert())
	const total = 500
	for i := 1; i <= total; i++ {
		seedTaskEvent(t, "t-500", 1, task.ID, "progress")
	}

	var buf bytes.Buffer
	streamTaskEvents(&buf, func() {}, task, context.Background(), 0)

	body := buf.String()
	// 无缺：progress 事件 500 条（加上 submitted 无、succeeded 无——本测试
	// 只播 progress；done 恰好 1 次）。
	require.Equal(t, total, strings.Count(body, "event: progress"), "progress 事件应无缺失")
	require.Equal(t, 1, strings.Count(body, "event: done"), "done 恰好一次")
	// 无重复：id 唯一（解析每行 id: N 集合大小 == 500）。
	ids := make(map[string]struct{})
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "id: ") {
			ids[strings.TrimSpace(line[4:])] = struct{}{}
		}
	}
	require.Len(t, ids, total, "id 集合应恰好 500 个（无重复）")
}
