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
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/model"
)

func setupTaskEventDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskEvent{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
	})
}

func TestRecordTaskEventAndList(t *testing.T) {
	setupTaskEventDB(t)
	ctx := context.Background()
	RecordTaskEvent(ctx, 42, "t-abc", 7, TaskEventSubmitted, TaskEventPayloadSubmitted{TaskID: "t-abc", Platform: "kling"})
	RecordTaskEvent(ctx, 42, "t-abc", 7, TaskEventQueued, TaskEventPayloadStatus{TaskID: "t-abc", Status: "QUEUED"})
	RecordTaskEvent(ctx, 42, "t-abc", 7, TaskEventSucceeded, TaskEventPayloadStatus{TaskID: "t-abc", Status: "SUCCESS"})

	events, err := model.ListTaskEventsAfter(42, 0, 100)
	require.NoError(t, err)
	require.Len(t, events, 3)
	// 升序
	assert.Less(t, events[0].ID, events[1].ID)
	assert.Less(t, events[1].ID, events[2].ID)
	assert.Equal(t, TaskEventSubmitted, events[0].Type)
	assert.Equal(t, TaskEventQueued, events[1].Type)
	assert.Equal(t, TaskEventSucceeded, events[2].Type)
	assert.Equal(t, int64(42), events[0].InternalTaskID)
	assert.Equal(t, "t-abc", events[0].TaskID)
	assert.Equal(t, 7, events[0].UserId)
	assert.NotEqual(t, int64(0), events[0].CreatedAt)
}

func TestListTaskEventsAfterSinceSeqNoGap(t *testing.T) {
	setupTaskEventDB(t)
	ctx := context.Background()
	types := []string{TaskEventSubmitted, TaskEventQueued, TaskEventClaimed, TaskEventProgress, TaskEventStepRatio, TaskEventSucceeded}
	for _, ty := range types {
		RecordTaskEvent(ctx, 100, "t-x", 1, ty, nil)
	}
	// 全部列出（since=0）
	all, err := model.ListTaskEventsAfter(100, 0, 100)
	require.NoError(t, err)
	require.Len(t, all, len(types))
	// 从第二条之后续传 → 只返回其后的事件（no gap）
	since := all[1].ID
	rest, err := model.ListTaskEventsAfter(100, since, 100)
	require.NoError(t, err)
	require.Len(t, rest, len(types)-2)
	assert.Equal(t, all[2].ID, rest[0].ID)
	// since=最后一条 → 空
	last, err := model.ListTaskEventsAfter(100, all[len(all)-1].ID, 100)
	require.NoError(t, err)
	assert.Empty(t, last)
}

func TestTaskEventTypesAllPersist(t *testing.T) {
	setupTaskEventDB(t)
	ctx := context.Background()
	allTypes := []string{
		TaskEventSubmitted, TaskEventQueued, TaskEventClaimed, TaskEventProgress,
		TaskEventStepRatio, TaskEventSucceeded, TaskEventFailed, TaskEventRefunded, TaskEventUnconfirmed,
	}
	for i, ty := range allTypes {
		RecordTaskEvent(ctx, int64(500+i), "t-"+ty, 1, ty, nil)
	}
	for i, ty := range allTypes {
		events, err := model.ListTaskEventsAfter(int64(500+i), 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, ty, events[0].Type)
	}
}

func TestRecordTaskEventNilDataBecomesEmptyObject(t *testing.T) {
	setupTaskEventDB(t)
	RecordTaskEvent(context.Background(), 600, "t-nil", 2, TaskEventQueued, nil)
	events, err := model.ListTaskEventsAfter(600, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "{}", string(events[0].Data))
}
