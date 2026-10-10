package common

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Batch-10 / G10 §12.2.2：后台 loop 心跳登记表的契约。
//
// 这张表存在的意义是让「某个常驻任务卡死」变成可观测；它自己**绝不能**成为
// 新的无界增长点 —— 因此键空间上界是被强制的，不是靠约定。

func resetLoopHeartbeats(t *testing.T) {
	t.Helper()
	loopHeartbeats.mu.Lock()
	loopHeartbeats.m = make(map[string]*atomic.Int64, 16)
	loopHeartbeats.mu.Unlock()
	loopHeartbeatOverflowWarned.Store(false)
	t.Cleanup(func() {
		loopHeartbeats.mu.Lock()
		loopHeartbeats.m = make(map[string]*atomic.Int64, 16)
		loopHeartbeats.mu.Unlock()
		loopHeartbeatOverflowWarned.Store(false)
	})
}

func TestLoopHeartbeatRecordsAndSnapshots(t *testing.T) {
	resetLoopHeartbeats(t)

	assert.Empty(t, LoopHeartbeatSnapshot(), "初始应为空")

	RecordLoopHeartbeat("sync_options")
	RecordLoopHeartbeat("consume_log_flusher")

	snap := LoopHeartbeatSnapshot()
	assert.Len(t, snap, 2)
	assert.Greater(t, snap["sync_options"], int64(0), "心跳应记录 Unix 毫秒时间戳")
	assert.Greater(t, snap["consume_log_flusher"], int64(0))
}

func TestLoopHeartbeatUpdatesExistingEntry(t *testing.T) {
	resetLoopHeartbeats(t)

	RecordLoopHeartbeat("sync_options")
	first := LoopHeartbeatSnapshot()["sync_options"]

	// 同一名字重复登记必须**复用**条目（而不是不断新增）
	for range 100 {
		RecordLoopHeartbeat("sync_options")
	}
	snap := LoopHeartbeatSnapshot()
	assert.Len(t, snap, 1, "重复登记同名 loop 不得增加条目")
	assert.GreaterOrEqual(t, snap["sync_options"], first)
}

func TestLoopHeartbeatIgnoresEmptyName(t *testing.T) {
	resetLoopHeartbeats(t)
	RecordLoopHeartbeat("")
	assert.Empty(t, LoopHeartbeatSnapshot(), "空名字必须被忽略（否则会造出一个以 \"\" 为键的条目）")
}

// 核心不变量：键空间**有硬上界** —— 即便有人误传业务键（用户 ID、请求 ID…），
// 也不会把这张表撑爆。
func TestLoopHeartbeatKeysAreBounded(t *testing.T) {
	resetLoopHeartbeats(t)

	total := LoopHeartbeatMaxEntries() * 3
	for i := range total {
		RecordLoopHeartbeat(fmt.Sprintf("loop-%d", i))
	}

	snap := LoopHeartbeatSnapshot()
	assert.LessOrEqual(t, len(snap), LoopHeartbeatMaxEntries(),
		"登记表条目数必须有硬上界（误用业务键时不得无界增长）")
	// 已登记的名字仍应能正常更新
	RecordLoopHeartbeat("loop-0")
	assert.Len(t, LoopHeartbeatSnapshot(), len(snap), "已登记的 loop 更新时不应新增条目")
}

func TestLoopHeartbeatSnapshotIsACopy(t *testing.T) {
	resetLoopHeartbeats(t)
	RecordLoopHeartbeat("sync_options")

	snap := LoopHeartbeatSnapshot()
	snap["injected"] = 1 // 修改返回值不得影响内部状态

	assert.NotContains(t, LoopHeartbeatSnapshot(), "injected")
}

func TestLoopHeartbeatIsConcurrencySafe(t *testing.T) {
	resetLoopHeartbeats(t)

	var wg sync.WaitGroup
	for w := range 16 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			name := fmt.Sprintf("loop-%d", worker%4)
			for range 200 {
				RecordLoopHeartbeat(name)
				_ = LoopHeartbeatSnapshot()
			}
		}(w)
	}
	wg.Wait()

	assert.Len(t, LoopHeartbeatSnapshot(), 4)
}
