package service

import (
	"sort"
	"sync"
	"time"
)

// LiveRequestTracker 进程内实时请求注册表：记录当前正在处理的 relay 请求，
// 供管理端「实时请求详情」面板展示（哪些请求进行中/排队/已完成、压缩前后
// 字节、已耗时、阶段等）。
//
// 设计取舍：
//   - 纯内存、无锁热点最小化（RWMutex + 每次操作 O(1) 摊销），不落库、不阻塞请求路径。
//   - 有界：最多保留 maxLiveRequests 条进行中记录，超出丢弃最旧（防极端并发下内存膨胀）。
//   - 已完成请求保留一小段「最近完成」窗口，便于面板显示刚处理完的请求。
//
// 多实例部署时各实例各自维护（面板按实例聚合），与既有 system_instances 一致。

const (
	maxLiveRequests      = 2000 // 进行中记录上限
	maxRecentlyFinished  = 200  // 最近完成保留条数
	finishedRetentionSec = 60   // 最近完成保留时长（秒）
)

// LiveRequestPhase 请求所处阶段。注意：全局并发排队发生在 relay 中间件层，
// 早于本注册表注册（LiveBegin），故不设单请求 waiting 阶段；排队数量见
// middleware.GetGlobalConcurrencyStats().Waiting（面板已展示 active/limit/waiting）。
type LiveRequestPhase string

const (
	LivePhaseReceived  LiveRequestPhase = "received"  // 已接收，尚未选中渠道
	LivePhaseUpstream  LiveRequestPhase = "upstream"  // 已发往上游，等待/接收响应
	LivePhaseStreaming LiveRequestPhase = "streaming" // 流式接收中（已收到首个有效 data 块）
	LivePhaseDone      LiveRequestPhase = "done"      // 已完成
	LivePhaseError     LiveRequestPhase = "error"     // 失败
)

// LiveRequestEntry 单个请求的实时快照。
type LiveRequestEntry struct {
	RequestId   string           `json:"request_id"`
	UserId      int              `json:"user_id"`
	UserName    string           `json:"user_name"`
	Model       string           `json:"model"`
	Group       string           `json:"group"`
	ChannelId   int              `json:"channel_id"`
	ChannelName string           `json:"channel_name"`
	IsStream    bool             `json:"is_stream"`
	Phase       LiveRequestPhase `json:"phase"`
	StartedAt   int64            `json:"started_at"`  // unix 毫秒（避免 elapsed 被量化到整秒）
	ElapsedMs   int64            `json:"elapsed_ms"`  // 快照时已耗时
	RetryIndex  int              `json:"retry_index"` // 第几次渠道尝试（0 起）

	// 请求体字节：OriginalBytes 是客户端原始大小；CompressedBytes 是压缩后发往
	// 上游的大小（未压缩时为 0，Compressed=false）。压缩节省可据此计算。
	OriginalBytes   int64 `json:"original_bytes"`
	CompressedBytes int64 `json:"compressed_bytes"`
	Compressed      bool  `json:"compressed"`
	// CompressionMs 本次压缩的**网关本地耗时**（毫秒，-1=未压缩）。用于区分
	// 「压缩慢」与「上传/上游慢」，做 AB 观测。
	CompressionMs int64 `json:"compression_ms"`

	// 首字（流式）/上游响应时间（毫秒）；0 表示尚未收到上游响应。
	FirstResponseMs int64 `json:"first_response_ms"`

	// 延迟拆解（毫秒，-1 表示该段未发生）。用于区分「网关→上游上传」与
	// 「上游首 token」，定位延迟卡在哪段。
	UpstreamConnectMs int64 `json:"upstream_connect_ms"`
	UpstreamUploadMs  int64 `json:"upstream_upload_ms"`
	UpstreamTtfbMs    int64 `json:"upstream_ttfb_ms"`

	// 终态字段（phase=done/error 时有效）。
	StatusCode int    `json:"status_code,omitempty"`
	ErrorMsg   string `json:"error_msg,omitempty"`
	FinishedAt int64  `json:"finished_at,omitempty"`
}

type liveRequestTracker struct {
	mu       sync.RWMutex
	active   map[string]*LiveRequestEntry
	finished []*LiveRequestEntry // 环形保留最近完成
	order    []string            // active 的插入顺序（用于超限淘汰最旧）
}

var liveRequests = &liveRequestTracker{
	active: make(map[string]*LiveRequestEntry),
}

// LiveBegin 注册一个新请求（请求进入 relay 处理时调用）。
func LiveBegin(reqId string, entry LiveRequestEntry) {
	if reqId == "" {
		return
	}
	entry.RequestId = reqId
	entry.Phase = LivePhaseReceived
	if entry.StartedAt == 0 {
		entry.StartedAt = time.Now().UnixMilli()
	}
	// 计时字段默认 -1（未采集），避免零值被当作「0ms」计入平均。
	if entry.UpstreamConnectMs == 0 {
		entry.UpstreamConnectMs = -1
	}
	if entry.UpstreamUploadMs == 0 {
		entry.UpstreamUploadMs = -1
	}
	if entry.UpstreamTtfbMs == 0 {
		entry.UpstreamTtfbMs = -1
	}
	if entry.CompressionMs == 0 {
		entry.CompressionMs = -1
	}
	liveRequests.mu.Lock()
	defer liveRequests.mu.Unlock()
	if _, exists := liveRequests.active[reqId]; !exists {
		liveRequests.order = append(liveRequests.order, reqId)
	}
	liveRequests.active[reqId] = &entry
	// 超限：淘汰最旧的进行中记录（正常情况下不会触发，防御极端并发）。
	for len(liveRequests.active) > maxLiveRequests && len(liveRequests.order) > 0 {
		oldest := liveRequests.order[0]
		liveRequests.order = liveRequests.order[1:]
		delete(liveRequests.active, oldest)
	}
}

// liveUpdate 以互斥方式就地更新一个进行中请求（不存在则忽略）。
func liveUpdate(reqId string, fn func(*LiveRequestEntry)) {
	if reqId == "" {
		return
	}
	liveRequests.mu.Lock()
	defer liveRequests.mu.Unlock()
	if e, ok := liveRequests.active[reqId]; ok {
		fn(e)
	}
}

// LiveSetChannel 记录选中的渠道与尝试序号。
func LiveSetChannel(reqId string, channelId int, channelName string, retryIndex int) {
	liveUpdate(reqId, func(e *LiveRequestEntry) {
		e.ChannelId = channelId
		e.ChannelName = channelName
		e.RetryIndex = retryIndex
		e.Phase = LivePhaseUpstream
	})
}

// LiveSetPhase 更新请求阶段（waiting/upstream/streaming）。
func LiveSetPhase(reqId string, phase LiveRequestPhase) {
	liveUpdate(reqId, func(e *LiveRequestEntry) { e.Phase = phase })
}

// LiveSetCompression 记录请求体压缩结果（原始字节 → 压缩后字节 + 压缩本地耗时）。
func LiveSetCompression(reqId string, originalBytes, compressedBytes, compressionMs int64, compressed bool) {
	liveUpdate(reqId, func(e *LiveRequestEntry) {
		e.OriginalBytes = originalBytes
		e.CompressedBytes = compressedBytes
		e.CompressionMs = compressionMs
		e.Compressed = compressed
	})
}

// LiveSetFirstResponse 记录首字/上游响应耗时（毫秒）。
func LiveSetFirstResponse(reqId string, firstResponseMs int64) {
	liveUpdate(reqId, func(e *LiveRequestEntry) { e.FirstResponseMs = firstResponseMs })
}

// LiveSetUpstreamTiming 记录延迟拆解：建连/上传/上游首字节耗时（毫秒）。
func LiveSetUpstreamTiming(reqId string, connectMs, uploadMs, ttfbMs int64) {
	liveUpdate(reqId, func(e *LiveRequestEntry) {
		e.UpstreamConnectMs = connectMs
		e.UpstreamUploadMs = uploadMs
		e.UpstreamTtfbMs = ttfbMs
	})
}

// LiveEnd 结束请求：移出进行中，进入最近完成窗口。
func LiveEnd(reqId string, statusCode int, errMsg string) {
	if reqId == "" {
		return
	}
	now := time.Now()
	liveRequests.mu.Lock()
	defer liveRequests.mu.Unlock()
	e, ok := liveRequests.active[reqId]
	if !ok {
		return
	}
	delete(liveRequests.active, reqId)
	for i, id := range liveRequests.order {
		if id == reqId {
			// [修复防御] 4.2.3：append 左移删除会令底层数组**尾部**残留对末元素的
			// 引用（strings/指针），使该对象无法被 GC —— 经典 slice 泄漏。显式把
			// 尾部槽位置空再截断。order 元素是 string（底层含指针），同属此风险。
			last := len(liveRequests.order) - 1
			copy(liveRequests.order[i:], liveRequests.order[i+1:])
			liveRequests.order[last] = ""
			liveRequests.order = liveRequests.order[:last]
			break
		}
	}
	e.FinishedAt = now.Unix()
	e.ElapsedMs = now.UnixMilli() - e.StartedAt
	e.StatusCode = statusCode
	e.ErrorMsg = errMsg
	if errMsg != "" || statusCode >= 400 {
		e.Phase = LivePhaseError
	} else {
		e.Phase = LivePhaseDone
	}
	liveRequests.finished = append(liveRequests.finished, e)
	// 修剪：按数量与时间双重限制。
	cutoff := now.Unix() - finishedRetentionSec
	kept := liveRequests.finished[:0]
	for _, f := range liveRequests.finished {
		if f.FinishedAt >= cutoff {
			kept = append(kept, f)
		}
	}
	liveRequests.finished = kept
	if len(liveRequests.finished) > maxRecentlyFinished {
		liveRequests.finished = liveRequests.finished[len(liveRequests.finished)-maxRecentlyFinished:]
	}
}

// LiveRequestsSnapshot 面板数据快照。
type LiveRequestsSnapshot struct {
	Active   []*LiveRequestEntry `json:"active"`
	Finished []*LiveRequestEntry `json:"finished"`
	// 聚合指标
	ActiveCount        int   `json:"active_count"`
	CompressedCount    int   `json:"compressed_count"`
	OriginalBytesSum   int64 `json:"original_bytes_sum"`
	CompressedBytesSum int64 `json:"compressed_bytes_sum"`
	// AvgCompressionRatio 压缩后/原始（仅统计已压缩请求；无数据时为 0）。
	AvgCompressionRatio float64 `json:"avg_compression_ratio"`
	// AvgFirstResponseMs 进行中+最近完成请求的平均首字（毫秒，仅统计已有首字的）。
	AvgFirstResponseMs int64 `json:"avg_first_response_ms"`
	// AvgUploadMs / AvgUpstreamTtfbMs 平均上传耗时 / 平均上游首字节耗时（毫秒，
	// 仅统计已采集到计时的请求）。用于区分「上传」与「上游」谁在拖慢首字。
	// 无数据时为 -1（未采集），调用方据此跳过展示，避免误显示「0ms」。
	AvgUploadMs       int64 `json:"avg_upload_ms"`
	AvgUpstreamTtfbMs int64 `json:"avg_upstream_ttfb_ms"`
	// AvgCompressionMs 平均压缩本地耗时（毫秒，仅统计已压缩请求）；无数据为 -1。
	AvgCompressionMs int64 `json:"avg_compression_ms"`
}

// GetLiveRequestsSnapshot 返回当前实时请求快照（含聚合）。
func GetLiveRequestsSnapshot() LiveRequestsSnapshot {
	now := time.Now()
	liveRequests.mu.RLock()
	defer liveRequests.mu.RUnlock()

	snap := LiveRequestsSnapshot{
		Active:   make([]*LiveRequestEntry, 0, len(liveRequests.active)),
		Finished: make([]*LiveRequestEntry, 0, len(liveRequests.finished)),
		// 聚合计时默认 -1（未采集），避免前端把「无数据」误显示为「0ms」。
		AvgUploadMs:       -1,
		AvgUpstreamTtfbMs: -1,
		AvgCompressionMs:  -1,
	}
	// 进行中：复制并刷新 elapsed（不修改共享对象）。
	for _, e := range liveRequests.active {
		cp := *e
		cp.ElapsedMs = now.UnixMilli() - e.StartedAt
		snap.Active = append(snap.Active, &cp)
	}
	sort.Slice(snap.Active, func(i, j int) bool {
		return snap.Active[i].StartedAt > snap.Active[j].StartedAt
	})
	// 最近完成：倒序（最新在前）。
	for i := len(liveRequests.finished) - 1; i >= 0; i-- {
		cp := *liveRequests.finished[i]
		snap.Finished = append(snap.Finished, &cp)
	}
	snap.ActiveCount = len(snap.Active)

	// 聚合：压缩率与平均首字（进行中 + 最近完成，避免只看到瞬时值）。
	var frtSum, frtCount int64
	var uploadSum, uploadCount int64
	var ttfbSum, ttfbCount int64
	var compSum, compCount int64
	accumulate := func(e *LiveRequestEntry) {
		if e.Compressed && e.OriginalBytes > 0 {
			snap.CompressedCount++
			snap.OriginalBytesSum += e.OriginalBytes
			snap.CompressedBytesSum += e.CompressedBytes
			if e.CompressionMs >= 0 {
				compSum += e.CompressionMs
				compCount++
			}
		}
		if e.FirstResponseMs > 0 {
			frtSum += e.FirstResponseMs
			frtCount++
		}
		if e.UpstreamUploadMs >= 0 {
			uploadSum += e.UpstreamUploadMs
			uploadCount++
		}
		if e.UpstreamTtfbMs >= 0 {
			ttfbSum += e.UpstreamTtfbMs
			ttfbCount++
		}
	}
	for _, e := range snap.Active {
		accumulate(e)
	}
	for _, e := range snap.Finished {
		accumulate(e)
	}
	if snap.OriginalBytesSum > 0 {
		snap.AvgCompressionRatio = float64(snap.CompressedBytesSum) / float64(snap.OriginalBytesSum)
	}
	if frtCount > 0 {
		snap.AvgFirstResponseMs = frtSum / frtCount
	}
	if uploadCount > 0 {
		snap.AvgUploadMs = uploadSum / uploadCount
	}
	if ttfbCount > 0 {
		snap.AvgUpstreamTtfbMs = ttfbSum / ttfbCount
	}
	if compCount > 0 {
		snap.AvgCompressionMs = compSum / compCount
	}
	return snap
}

// ResetLiveRequestsForTest 清空注册表（仅测试用）。
func ResetLiveRequestsForTest() {
	liveRequests.mu.Lock()
	defer liveRequests.mu.Unlock()
	liveRequests.active = make(map[string]*LiveRequestEntry)
	liveRequests.finished = nil
	liveRequests.order = nil
}
