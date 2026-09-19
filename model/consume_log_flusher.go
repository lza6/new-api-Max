package model

import (
	"context"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 消费/错误日志异步批量落库：把热路径的同步 INSERT 移出请求返回链路，
// 后台按批/间隔批量写库（对标 sub2api 用量异步 flusher）。
// 权衡：进程崩溃最多丢失一个批次的日志（WAL 保证已提交数据；计费流水保持同步不动）。
var (
	consumeLogQueue    = make(chan *Log, 4096)
	consumeFlushOnce   sync.Once
	consumeFlushCancel context.CancelFunc
	consumeFlushWG     sync.WaitGroup
)

// enqueueAsyncLog 异步入队；队列满时降级为同步写（背压，不丢数据）。
func enqueueAsyncLog(log *Log) {
	select {
	case consumeLogQueue <- log:
	default:
		if err := createLog(log); err != nil {
			common.SysError("failed to record log (queue full fallback): " + err.Error())
		}
	}
}

// StartConsumeLogFlusher 启动后台批量落库 worker（进程内单例，幂等）。
func StartConsumeLogFlusher() {
	consumeFlushOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		consumeFlushCancel = cancel
		consumeFlushWG.Add(1)
		go consumeLogFlusherLoop(ctx)
	})
}

func consumeLogFlusherLoop(ctx context.Context) {
	defer consumeFlushWG.Done()
	interval := time.Duration(common.LogFlushIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}
	batchSize := common.LogFlushBatch
	if batchSize <= 0 {
		batchSize = 500
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var pending []*Log
	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = nil
		if err := LOG_DB.CreateInBatches(batch, 200).Error; err != nil {
			common.SysError("failed to batch insert logs: " + err.Error())
		}
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case log := <-consumeLogQueue:
			pending = append(pending, log)
			if len(pending) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// FlushConsumeLogs 同步排空队列（测试/运维/优雅退出用；仅统计已入队的日志）。
func FlushConsumeLogs() {
	for {
		select {
		case log := <-consumeLogQueue:
			if err := createLog(log); err != nil {
				common.SysError("failed to record log (flush): " + err.Error())
			}
		default:
			return
		}
	}
}

// StopConsumeLogFlusher 停止后台 worker 并排空（供测试/优雅退出调用）。
func StopConsumeLogFlusher() {
	if consumeFlushCancel != nil {
		consumeFlushCancel()
		consumeFlushWG.Wait()
		consumeFlushCancel = nil
	}
	FlushConsumeLogs()
}
