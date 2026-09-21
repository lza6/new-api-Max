package model

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 消费/错误日志异步批量落库：把热路径的同步 INSERT 移出请求返回链路，
// 后台按批/间隔批量写库（对标 sub2api 用量异步 flusher）。
// 权衡：进程崩溃最多丢失一个批次的日志（WAL 保证已提交数据；计费流水保持同步不动）。
var (
	consumeLogQueue    = make(chan *Log, 4096)
	consumeFlushMu     sync.Mutex
	consumeFlushCancel context.CancelFunc
	consumeFlushWG     sync.WaitGroup
	// P1-2 指标：队列深度/批量/失败（读端 GetConsumeLogFlusherMetrics）。
	consumeFlushMetrics consumeLogFlusherMetrics
)

// consumeLogFlusherMetrics 批量落库运行指标（原子，供 perf_metrics/管理台）。
type consumeLogFlusherMetrics struct {
	QueueDepth    atomic.Int64 // 当前队列长度
	LastBatchSize atomic.Int64 // 最近一次批量写入行数
	Failures      atomic.Int64 // 批量写入失败次数（重试后仍失败）
	Retries       atomic.Int64 // 批量写入失败后重试次数
}

// GetConsumeLogFlusherMetrics 返回批量落库指标快照。
func GetConsumeLogFlusherMetrics() (queueDepth, lastBatchSize, failures, retries int64) {
	return consumeFlushMetrics.QueueDepth.Load(),
		consumeFlushMetrics.LastBatchSize.Load(),
		consumeFlushMetrics.Failures.Load(),
		consumeFlushMetrics.Retries.Load()
}

// enqueueAsyncLog 异步入队；队列满时降级为同步写（背压，不丢数据）。
func enqueueAsyncLog(log *Log) {
	select {
	case consumeLogQueue <- log:
		consumeFlushMetrics.QueueDepth.Store(int64(len(consumeLogQueue)))
	default:
		if err := createLog(log); err != nil {
			common.SysError("failed to record log (queue full fallback): " + err.Error())
		}
	}
}

// StartConsumeLogFlusher 启动后台批量落库 worker（进程内单例，幂等）。
// 停止后可再次启动（测试/优雅退出后重启场景）；并发启动只生效一次。
func StartConsumeLogFlusher() {
	consumeFlushMu.Lock()
	defer consumeFlushMu.Unlock()
	if consumeFlushCancel != nil {
		return // 已在运行
	}
	ctx, cancel := context.WithCancel(context.Background())
	consumeFlushCancel = cancel
	consumeFlushWG.Add(1)
	go consumeLogFlusherLoop(ctx)
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
		// P1-2 失败语义：批写失败先整批重试一次（瞬时错误）；仍失败则
		// 逐行回退——坏行丢弃并告警，好行保住（计费日志不丢），绝不
		// panic/crash。
		if err := LOG_DB.CreateInBatches(batch, 200).Error; err != nil {
			consumeFlushMetrics.Retries.Add(1)
			if retryErr := LOG_DB.CreateInBatches(batch, 200).Error; retryErr != nil {
				consumeFlushMetrics.Failures.Add(1)
				common.SysError("batch insert failed (after retry), falling back row-by-row: " + retryErr.Error())
				salvaged := 0
				for _, l := range batch {
					if rowErr := createLog(l); rowErr == nil {
						salvaged++
					}
				}
				if salvaged < len(batch) {
					common.SysError("row-by-row fallback salvaged " + strconv.Itoa(salvaged) + "/" + strconv.Itoa(len(batch)) + " logs")
				}
				return
			}
		}
		consumeFlushMetrics.LastBatchSize.Store(int64(len(batch)))
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case log := <-consumeLogQueue:
			pending = append(pending, log)
			consumeFlushMetrics.QueueDepth.Store(int64(len(consumeLogQueue)))
			if len(pending) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// FlushConsumeLogs 同步排空队列（测试/运维/优雅退出用）。
// 先暂停后台 worker，避免与批量 pending 竞争导致部分日志滞留到下一 tick；
// 排空期间入队的新日志也一并落库（循环到队列空）。
func FlushConsumeLogs() {
	// 暂停 worker（若在运行），排空后由调用方决定是否重启。
	consumeFlushMu.Lock()
	cancel := consumeFlushCancel
	consumeFlushMu.Unlock()
	if cancel != nil {
		cancel()
		consumeFlushWG.Wait()
		consumeFlushMu.Lock()
		consumeFlushCancel = nil
		consumeFlushMu.Unlock()
	}
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
// 与 Start 配对（mutex 保护），停止后可安全重启。
func StopConsumeLogFlusher() {
	consumeFlushMu.Lock()
	cancel := consumeFlushCancel
	consumeFlushMu.Unlock()
	if cancel != nil {
		cancel()
		consumeFlushWG.Wait()
		consumeFlushMu.Lock()
		consumeFlushCancel = nil
		consumeFlushMu.Unlock()
	}
	FlushConsumeLogs()
}
