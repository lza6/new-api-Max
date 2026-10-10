package common

import (
	"sync"
	"sync/atomic"
	"time"
)

// 后台常驻 loop 的**心跳登记表**（Batch-10 / G10 §12.2.2）。
//
// 解决的问题：项目有一批 `for { ... time.Sleep(...) }` 常驻循环（选项同步、系统任务
// 调度、订阅额度重置、产物清理、凭证刷新、消费日志落库…）。它们卡死时**没有任何信号**
// —— 只能靠翻日志猜。这里让每个 loop 在自己的 tick 末尾打一次时间戳，
// 由 `/metrics` 输出为 `background_loop_last_run_timestamp_seconds{loop="…"}`：
// 某条曲线不再前进 = 那个 loop 卡住了，一眼可见。
//
// 无界增长风险：键是**编译期常量**（每个 loop 一个固定名字），条目数 = 接入的 loop 数量，
// 有界且不随流量变化。这也是本文件刻意用「调用方传固定字符串」而不是「按业务键登记」的原因。

type loopHeartbeatTable struct {
	mu sync.RWMutex
	m  map[string]*atomic.Int64 // name -> last run unix millis
}

var loopHeartbeats = loopHeartbeatTable{m: make(map[string]*atomic.Int64, 16)}

// loopHeartbeatMaxEntries 是登记表条目**硬上界**。
//
// 上面的「name 必须是编译期常量」是约定；这里把它变成**被强制的**上界：
// 超过则拒绝登记并告警一次。这样即便将来有人误把业务键（如用户 ID）传进来，
// 也不会把这张表变成新的无界增长点 —— 本批次修的正是这一类问题。
const loopHeartbeatMaxEntries = 128

var loopHeartbeatOverflowWarned atomic.Bool

// LoopHeartbeatMaxEntries 暴露上界供测试断言。
func LoopHeartbeatMaxEntries() int { return loopHeartbeatMaxEntries }

// RecordLoopHeartbeat 记录某个后台 loop 刚刚跑完一轮。
// name 必须是**编译期常量**（如 "sync_options"）—— 用外部输入当名字会让这张表无界增长，
// 因此超过上界后会被拒绝。
func RecordLoopHeartbeat(name string) {
	if name == "" {
		return
	}
	loopHeartbeats.mu.RLock()
	cell, ok := loopHeartbeats.m[name]
	loopHeartbeats.mu.RUnlock()
	if ok {
		cell.Store(time.Now().UnixMilli())
		return
	}

	loopHeartbeats.mu.Lock()
	defer loopHeartbeats.mu.Unlock()
	if cell, ok := loopHeartbeats.m[name]; ok {
		cell.Store(time.Now().UnixMilli())
		return
	}
	if len(loopHeartbeats.m) >= loopHeartbeatMaxEntries {
		if loopHeartbeatOverflowWarned.CompareAndSwap(false, true) {
			SysError("loop heartbeat registry is full; refusing to register new loop name (this is a programming error)")
		}
		return
	}
	cell = &atomic.Int64{}
	cell.Store(time.Now().UnixMilli())
	loopHeartbeats.m[name] = cell
}

// LoopHeartbeatSnapshot 返回全部 loop 的上次执行时刻（Unix 毫秒）副本。
func LoopHeartbeatSnapshot() map[string]int64 {
	loopHeartbeats.mu.RLock()
	defer loopHeartbeats.mu.RUnlock()
	out := make(map[string]int64, len(loopHeartbeats.m))
	for name, cell := range loopHeartbeats.m {
		out[name] = cell.Load()
	}
	return out
}
