package training_capture

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// 采集 worker：消费队列 → 组装目录 → 取增量 → 分片上传。
//
// ## 为什么是**单** goroutine
//
// 1. **并发写同一分片会交错**：同一用户同一模型的两个请求若并发上传，
//    分片序号会冲突或内容交错。单 worker 让「序号分配 → 上传」原子化。
// 2. 上传是**带宽受限**的（本站上行 75Mbps）。多 worker 只会让多个上传
//    互相抢带宽，用户请求跟着变慢——正是要避免的。
// 3. 采集是「尽力而为」的辅助功能，吞吐不是关键指标；
//    真正需要的是**不影响主业务** + **不写坏数据**。
//
// 若未来确实需要更高吞吐，正确的扩展方向是「按用户分片的多 worker」
// （同一用户仍由同一 worker 处理），而不是无脑加并发。
//
// ## 自适应（用户明确要求）
//
// 用**令牌桶限速**控制上传字节速率：上行带宽是全站共享的，
// 采集打满上行会让**所有用户**的请求变慢。默认限速取一个保守值，
// 可配。队列积压时也不加速（宁可丢采集，不抢用户带宽）。

const (
	// uploadRateLimitBytesPerSec 采集上传限速（字节/秒）。
	//
	// 本站上行 75Mbps ≈ 9.4MB/s。取 **1MB/s** 作为采集上限：
	// 约占总上行 10.6%，即使采集满速也留有近 90% 给用户请求。
	// 用户明确要求「自适应」「不能影响请求延迟」，保守取值是正确的取舍。
	uploadRateLimitBytesPerSec = 1 << 20

	// workerIdleSleep 队列空时的等待时长。
	//
	// **不用忙轮询**：空队列时睡 200ms，CPU 占用可忽略。
	// 采集对实时性要求低（用户感知不到几百毫秒的延迟）。
	workerIdleSleep = 200 * time.Millisecond

	// workerErrorBackoff 上传失败后的退避基数（指数增长）。
	workerErrorBackoff = 2 * time.Second

	// workerMaxBackoff 退避上限。
	workerMaxBackoff = 2 * time.Minute

	// workerMaxConsecutiveErrors 连续失败多少次后进入长时间退避。
	//
	// 云盘持续不可用时，重试只是在浪费资源并刷日志
	// （用户要的是「不影响主业务」，不是「拼命重试」）。
	workerMaxConsecutiveErrors = 5

	// MaxPartBytes 单个分片的目标大小。
	//
	// 必须 < `OneDriveSimpleUploadMaxBytes`(4MB)，否则会走上传会话
	// （另一套复杂逻辑）。取 2MB：既减少文件数，又留足余量。
	// **这是「文件不会无限增大」的实现**（用户明确要求轮转）。
	MaxPartBytes = 2 << 20

	// watermarkCleanupAge 水位清理阈值：超过该时长未更新的水位被删。
	//
	// 水位是会话级状态，会话长期不活跃后价值归零。
	// 删了只会让该会话下次重新全量采集，**不丢数据**。
	watermarkCleanupAge = 30 * 24 * time.Hour
)

// CaptureStats 采集统计（进程内原子计数，供管理端实时读）。
//
// **今日口径**：按 UTC 日期切换。用 UTC 与目录命名一致
// （见 `DateDirName` 的说明），避免「展示的今日」与「目录里的今日」不一致。
type CaptureStats struct {
	// 当前统计所属的 UTC 日期（YYYY-MM-DD）。跨日时自动重置。
	Day atomic.Pointer[string]

	// Users 今日被采集过的**去重用户数**。
	Users atomic.Int64
	// Conversations 今日被采集过的对话数（累计上传次数，非去重）。
	Conversations atomic.Int64
	// BytesUploaded 今日上传字节数。
	BytesUploaded atomic.Int64

	// InFlightBytes 当前**正在上传**的字节数（增量需求 B5）。
	// 管理端据此显示「正在上传 X MB」——只看累计值看不出「现在在忙什么」。
	// 用原子加减配对（worker 单 goroutine，但仍保持并发安全语义）。
	InFlightBytes atomic.Int64
	// MessagesUploaded 今日上传的消息条数。
	MessagesUploaded atomic.Int64

	// TotalEnqueued 累计投递（进程启动以来）。
	TotalEnqueued atomic.Int64
	// TotalDropped 累计丢弃（队列满）。
	TotalDropped atomic.Int64
	// TotalUploadErrors 累计上传失败次数。
	TotalUploadErrors atomic.Int64
	// TotalSkippedNoConsent 因未授权而跳过的次数。
	TotalSkippedNoConsent atomic.Int64

	// TotalSkippedNoText 因**取不到任何文本**而跳过的次数。
	//
	// 为什么必须单独计数（而不是静默 return nil）：当 CountToken 关闭时
	// 上游 meta.CombineText 为空，采集会一条不出 —— 而管理端只看得到
	// 「今天 0 条」，无法区分「没流量」与「配置导致的静默失效」。
	// 这个计数器就是那个区分信号（盲点扫描发现的可观测性缺口）。
	TotalSkippedNoText atomic.Int64

	// usersToday 今日去重用户集合（有界，只存今日）。
	usersTodayMu sync.Mutex
	usersToday   map[int]struct{}
}

var captureStats = &CaptureStats{}

func init() {
	today := timeUTCDate(time.Now().Unix())
	captureStats.Day.Store(&today)
	captureStats.usersToday = make(map[int]struct{}, 1024)
}

// CaptureStatsSnapshot 统计快照（供接口层序列化）。
type CaptureStatsSnapshot struct {
	Day               string `json:"day"`
	Users             int64  `json:"users"`
	Conversations     int64  `json:"conversations"`
	BytesUploaded     int64  `json:"bytes_uploaded"`
	MessagesUploaded  int64  `json:"messages_uploaded"`
	TotalEnqueued     int64  `json:"total_enqueued"`
	TotalDropped      int64  `json:"total_dropped"`
	TotalUploadErrors int64  `json:"total_upload_errors"`
	SkippedNoConsent  int64  `json:"skipped_no_consent"`
	SkippedNoText     int64  `json:"skipped_no_text"`
	QueueLen          int    `json:"queue_len"`
	// InFlightBytes 当前正在上传的字节数（瞬时值）。
	InFlightBytes int64 `json:"in_flight_bytes"`
	QueueCapacity int   `json:"queue_capacity"`
}

// GetCaptureStats 返回统计快照。
func GetCaptureStats() CaptureStatsSnapshot {
	day := ""
	if p := captureStats.Day.Load(); p != nil {
		day = *p
	}
	queueStats := captureQueue.Stats()
	return CaptureStatsSnapshot{
		Day:               day,
		Users:             captureStats.Users.Load(),
		Conversations:     captureStats.Conversations.Load(),
		BytesUploaded:     captureStats.BytesUploaded.Load(),
		MessagesUploaded:  captureStats.MessagesUploaded.Load(),
		TotalEnqueued:     captureStats.TotalEnqueued.Load(),
		TotalDropped:      captureStats.TotalDropped.Load(),
		TotalUploadErrors: captureStats.TotalUploadErrors.Load(),
		SkippedNoConsent:  captureStats.TotalSkippedNoConsent.Load(),
		SkippedNoText:     captureStats.TotalSkippedNoText.Load(),
		QueueLen:          queueStats.Len,
		InFlightBytes:     captureStats.InFlightBytes.Load(),
		QueueCapacity:     queueStats.Capacity,
	}
}

// rolloverDayIfNeeded 跨 UTC 日时重置「今日」计数。
func rolloverDayIfNeeded(now time.Time) {
	today := timeUTCDate(now.Unix())
	p := captureStats.Day.Load()
	if p != nil && *p == today {
		return
	}
	// 双检：并发下可能多个 goroutine 同时进来，重置多次也无害
	// （只是把计数器清零）。用 CAS 让「只有第一个」执行重置。
	if captureStats.Day.CompareAndSwap(p, &today) {
		captureStats.Users.Store(0)
		captureStats.Conversations.Store(0)
		captureStats.BytesUploaded.Store(0)
		captureStats.MessagesUploaded.Store(0)
		captureStats.usersTodayMu.Lock()
		captureStats.usersToday = make(map[int]struct{}, 1024)
		captureStats.usersTodayMu.Unlock()
	}
}

func markUserToday(userId int) {
	captureStats.usersTodayMu.Lock()
	if _, ok := captureStats.usersToday[userId]; !ok {
		captureStats.usersToday[userId] = struct{}{}
		captureStats.Users.Store(int64(len(captureStats.usersToday)))
	}
	captureStats.usersTodayMu.Unlock()
}

// ---------- 队列与 worker 生命周期 ----------

var (
	captureQueue = newAdaptiveCaptureQueue()

	workerOnce sync.Once
	workerStop chan struct{}
)

// newAdaptiveCaptureQueue 按内存预算构造队列（增量需求 B1）。
//
// 预算在 initResourceBudget（启动装配调用）里确定；
// 未初始化时退回默认容量 —— 保证「忘记接线」时行为与旧版一致，
// 而不是 panic 或容量 0。
func newAdaptiveCaptureQueue() *CaptureQueue {
	return NewCaptureQueue(GetResourceBudget().QueueCapacity)
}

// MaybeCapture 是**请求路径上唯一被调用的入口**。
//
// 契约（宪法 V 的落实）：
//   - 非阻塞：队列满立即返回，绝不等待
//   - 无 DB 写、无网络、无锁竞争（授权判定走缓存）
//   - 不做 JSON 解析（调用方传已算好的数据）
//
// 返回 false 表示未入队（未授权 / 总开关关 / 队列满）。
func MaybeCapture(signal CaptureSignal) bool {
	if signal.UserID <= 0 || len(signal.Messages) == 0 {
		return false
	}
	// 授权判定：命中缓存时是一次 map 读。
	if !IsGranted(signal.UserID) {
		captureStats.TotalSkippedNoConsent.Add(1)
		return false
	}
	if signal.CreatedAt == 0 {
		signal.CreatedAt = time.Now().Unix()
	}
	ok := captureQueue.TryEnqueue(signal)
	if ok {
		captureStats.TotalEnqueued.Add(1)
	} else {
		captureStats.TotalDropped.Add(1)
	}
	return ok
}

// StartWorker 启动采集 worker（幂等；多次调用只启动一个）。
func StartWorker() {
	workerOnce.Do(func() {
		workerStop = make(chan struct{})
		go runWorker()
	})
}

// StopWorker 停止采集 worker（供总开关关闭 / 停机）。
//
// 通过关闭队列让 `Receive` 返回 false 从而退出循环——
// 不丢已入队的数据（关闭后仍会取完缓冲）。
func StopWorker() {
	if workerStop != nil {
		select {
		case <-workerStop:
		default:
			close(workerStop)
		}
	}
}

// runWorker 主循环。
func runWorker() {
	// [修复防御] **顶层 panic 恢复是必须的，不是可选加固**。
	//
	// 后台 goroutine 的 panic 在 Go 里**不会**被 net/http 的 per-connection
	// recover 兜住 —— 它直接终止**整个网关进程**。
	// 而本函数调用链里有网络（sink 写入）、gzip、JSON 解析、DB、字符串切片，
	// 任一处对畸形数据 panic，触发者都只是一条普通用户请求。
	//
	// 这与本模块的核心承诺（「采集绝不拖垮主业务」）完全相反，
	// 且排查极难：崩的是后台 goroutine，没有请求栈可看。
	// 因此这里把「采到坏数据」与「网关宕机」彻底解耦。
	defer func() {
		if r := recover(); r != nil {
			captureStats.TotalUploadErrors.Add(1)
			common.SysError(fmt.Sprintf(
				"training capture: worker panic recovered (capture continues, gateway unaffected): %v", r))
			// 不重入循环：本次处理的那条数据放弃，避免在坏数据上死循环。
			// worker 由调用方（StartWorker/管理端开关）负责重启。
		}
	}()

	// 把 workerStop 绑进限速器：停机时限速等待也要被打断，
	// 否则一个正在等 2 分钟的限速会让 StopWorker 卡住。
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, workerStop)
	consecutiveErrors := 0

	for {
		select {
		case <-workerStop:
			return
		default:
		}

		// 把 workerStop 一并交给 Receive：否则 worker 阻塞在这里等数据时
		// StopWorker 无法唤醒它（两个是不同的 channel，见 ReceiveWithStop）。
		signal, ok := captureQueue.ReceiveWithStop(workerStop)
		if !ok {
			// 队列关闭（停机）或暂无数据。Receive 是阻塞的，
			// 不会走到这里空转；保险起见仍睡一下。
			select {
			case <-workerStop:
				return
			case <-time.After(workerIdleSleep):
			}
			continue
		}

		// 消费时**再次**检查总开关：管理员可能在信号入队后关闭了采集。
		// 不检查会让「关闭后仍在传」出现一个队列长度的窗口。
		if !IsGatewayEnabled() || !CanEnableGateway() {
			continue
		}

		// 消费时**再次**检查该用户是否仍授权。
		//
		// [修复防御] 授权判定发生在入队时（`MaybeCapture` → `IsGranted`），
		// 但从入队到被消费之间有**队列积压窗口**（高并发下可达数秒）。
		// 用户在这个窗口内撤回时，信号已经在队列里了 —— 只靠入队时的判定
		// 会让他「撤回后仍被上传」。
		//
		// `IsGranted` 命中缓存时只是一次 map 读 + TTL 比对（实测 ~1µs），
		// 在 worker 侧（非请求路径）完全可接受；而撤回会主动失效缓存，
		// 所以这里能立刻看到最新状态。
		if !IsGranted(signal.UserID) {
			captureStats.TotalSkippedNoConsent.Add(1)
			continue
		}

		uploaded, uploadedBytes, err := processSignal(context.Background(), signal, limiter)
		if err != nil {
			captureStats.TotalUploadErrors.Add(1)
			consecutiveErrors++
			backoff := workerErrorBackoff * time.Duration(1<<min(consecutiveErrors, 6))
			if backoff > workerMaxBackoff {
				backoff = workerMaxBackoff
			}
			logWorkerError(signal, err, consecutiveErrors)
			if consecutiveErrors >= workerMaxConsecutiveErrors {
				// 连续失败 → 长退避（云盘可能长期不可用；
				// 拼命重试只会刷日志、占资源）。
				backoff = workerMaxBackoff
			}
			select {
			case <-workerStop:
				return
			case <-time.After(backoff):
			}
			continue
		}
		consecutiveErrors = 0

		if uploaded > 0 {
			now := time.Now()
			rolloverDayIfNeeded(now)
			markUserToday(signal.UserID)
			captureStats.Conversations.Add(1)
			captureStats.MessagesUploaded.Add(int64(uploaded))
			captureStats.BytesUploaded.Add(uploadedBytes)
		}
	}
}

func logWorkerError(signal CaptureSignal, err error, consecutive int) {
	// 日志**不含对话内容**（隐私），只记定位所需的最小信息。
	common.SysError(fmt.Sprintf(
		"training capture: upload failed (user=%d model=%s consecutive=%d): %s",
		signal.UserID, signal.Model, consecutive, err.Error()))
}

// processSignal 处理一条信号：取增量 → 组装路径 → 分片 → 上传。
// 返回 (上传的消息条数, 上传字节数, error)。
func processSignal(ctx context.Context, signal CaptureSignal, limiter *byteLimiter) (int, int64, error) {
	if signal.UserID <= 0 || len(signal.Messages) == 0 {
		return 0, 0, nil
	}

	// 组装完整消息序列：请求里的对话 + 助手回复。
	// 助手回复作为最后一条 assistant 消息（可能为空——见 plan §10.3 的诚实边界）。
	messages := make([]Message, 0, len(signal.Messages)+1)
	messages = append(messages, signal.Messages...)
	if strings.TrimSpace(signal.ResponseText) != "" {
		messages = append(messages, Message{Role: "assistant", Content: signal.ResponseText})
	}

	conversationId := strings.TrimSpace(signal.ConversationID)

	var increment []Message
	var nextWatermark Watermark
	var nextTextWatermark TextWatermark
	var textSuffix string

	if conversationId != "" {
		// 主路径：会话水位精确增量。
		wm, err := model.GetTrainingDataWatermark(signal.UserID, conversationId)
		if err != nil {
			return 0, 0, fmt.Errorf("load watermark: %w", err)
		}
		var current Watermark
		var currentText TextWatermark
		if wm != nil {
			current = Watermark{
				UploadedCount:     wm.UploadedCount,
				PrefixFingerprint: wm.PrefixFingerprint,
			}
			currentText = TextWatermark{
				UploadedBytes: wm.RequestTextBytes,
				PrefixHash:    wm.RequestTextHash,
			}
		}

		// 数据形态判定（实测确认，见 text_increment.go）：
		// 请求路径只有 1 条 `role="request"`（拼接文本）+ 可能 1 条 assistant。
		// 这种情况下**必须**按文本前缀做增量 —— 消息级水位恒为 1，
		// 去重完全失效（长期会话每次都全量重传，正是用户要避免的）。
		//
		// 保留消息级路径是为了真实结构化消息（将来扩展 dto.Request 后）
		// 仍走原来的、更强的逐条语义。
		if isSingleRequestBlob(messages) {
			// 超限截断的会话：新内容在**尾部**，而存储文本含「省略 N 字节」
			// 标记（N 每轮都变）。对整段做前缀比对会退化成每次全量重传，
			// 所以这里改用 `TailOnly` 做真正的尾部增量。
			//
			// 这修复了一个静默丢数据的缺陷：原实现对超限会话只保留头部，
			// 前缀恒定 → 增量后缀恒为空 → **尾部新内容永远不被采集**。
			if signal.TailOnly != "" {
				textSuffix, nextTextWatermark = SelectTextIncrement(signal.TailOnly, currentText)
				if textSuffix != "" {
					increment = []Message{{Role: "request", Content: textSuffix}}
				}
			} else {
				textSuffix, nextTextWatermark = SelectTextIncrement(messages[0].Content, currentText)
				if textSuffix != "" {
					increment = []Message{{Role: "request", Content: textSuffix}}
				}
			}
		} else {
			increment, nextWatermark = SelectIncrement(messages, current)
		}
	} else {
		// 退化路径：拿不到会话 id 时按内容指纹全局去重。
		increment = GlobalDedup(messages, globalSeenFingerprints)
	}

	if len(increment) == 0 {
		return 0, 0, nil
	}

	// 目录：日期 / 用户名 / 模型
	day := DateDirName(signal.CreatedAt)
	userDir := SanitizeDirName(signal.UserID, signal.Username)
	modelDir := SanitizeModelName(signal.Model)
	dirPath := day + "/" + userDir + "/" + modelDir

	sink := ActiveSink()
	if err := sink.EnsureDir(ctx, dirPath); err != nil {
		return 0, 0, fmt.Errorf("ensure dir: %w", err)
	}

	// 分片：列举现有分片 → 取下一个序号 → 追加内容 → 超限开新分片。
	existing, err := sink.List(ctx, dirPath)
	if err != nil {
		return 0, 0, fmt.Errorf("list parts: %w", err)
	}
	partSeq := PickNextPartSeq(existing)

	payload := encodeJSONL(increment)

	// ## 切分与压缩的顺序（这里曾经写错过，是个数据损坏级 bug）
	//
	// **必须先把原始 JSONL 按行切分，再对每一块独立 gzip。**
	//
	// 曾经的做法是先 `gzipCompress(整个 payload)` 得到**一个** gzip member，
	// 再把这个 member 的字节按大小切开 —— 那是错的：
	// 一个 gzip member 在任意字节位置切开，得到的是**碎片**，
	// 每个碎片都不是合法的 gzip（`unexpected EOF` / `invalid header`）。
	// 后果不是报错而是**静默乱码**：预览按 gzip magic 判断失败后当明文显示
	// 二进制垃圾，分析端则把碎片当明文喂给模型。
	//
	// 正确顺序：原始文本按行切 → 每块各自 gzip → 每个分片都是**完整 member**，
	// 可独立解压。这与 compression.go 的契约、以及
	// compression_test.go「截断的 gzip 必须报错」的断言一致。
	rawChunkSize := EffectiveMaxPartRawBytes()
	// 按行边界切，保证不把一条 JSON 记录切成两半。
	rawChunks := splitBySize(payload, rawChunkSize)

	var uploadedBytes int64
	// written 记录**本批**已成功写入的分片，用于失败时补偿删除。
	//
	// [修复防御] 没有它会出现「部分成功」残留（盲点扫描发现）：
	// 逐块上传时若第 2 块失败，第 1 块已落盘 → 水位不推进（对，不丢数据），
	// 但下轮重试会 List 到更大的 seq，把**整段增量再写一遍** →
	// 云盘上出现重复数据 + 孤儿分片，且孤儿会被分析/导出当有效增量读回。
	var written []string
	for _, rawChunk := range rawChunks {
		partName := dirPath + "/" + PartFileName(partSeq)

		// 对**这一块**独立压缩（每个分片 = 一个完整 gzip member）。
		blob, err := gzipCompress(rawChunk)
		if err != nil {
			// 压缩失败极罕见（内存不足等）。降级为明文上传 —— 宁可多占带宽
			// 也不丢数据；同时记录，便于发现系统性问题。
			common.SysError("training capture: gzip failed, uploading plain: " + err.Error())
			blob = rawChunk
		}
		// 压缩率意外差时（已压缩内容/随机数据）仍可能超过单次上传上限：
		// 那时**再切一刀**并各自重压，而不是把超限数据硬塞进去。
		if len(blob) > OneDriveSimpleUploadMaxBytes-64<<10 {
			for _, sub := range splitBySize(blob, OneDriveSimpleUploadMaxBytes-64<<10) {
				subName := dirPath + "/" + PartFileName(partSeq)
				limiter.wait(ctx, int64(len(sub)))
				captureStats.InFlightBytes.Add(int64(len(sub)))
				_, writeErr := sink.WritePart(ctx, subName, sub)
				captureStats.InFlightBytes.Add(-int64(len(sub)))
				if writeErr != nil {
					cleanupOrphanParts(ctx, sink, written)
					return 0, 0, fmt.Errorf("write part %s: %w", subName, writeErr)
				}
				written = append(written, subName)
				uploadedBytes += int64(len(sub))
				partSeq++
			}
			continue
		}

		// 自适应限速：按本次上传字节数等待额度。
		limiter.wait(ctx, int64(len(blob)))
		// 在传字节实时可见（B5）：管理端要能看出「现在在忙什么」。
		// 必须在 WritePart 前后成对加减 —— 忘记减会让数字只涨不落，
		// 管理员会以为卡死。
		captureStats.InFlightBytes.Add(int64(len(blob)))
		_, writeErr := sink.WritePart(ctx, partName, blob)
		captureStats.InFlightBytes.Add(-int64(len(blob)))
		if writeErr != nil {
			cleanupOrphanParts(ctx, sink, written)
			// 返回 0 字节：已上传的部分被删除，本轮等于什么都没发生，
			// 水位不推进 → 下轮重试整段，数据集不会出现半截或重复。
			return 0, 0, fmt.Errorf("write part %s: %w", partName, writeErr)
		}
		written = append(written, partName)
		uploadedBytes += int64(len(blob))
		partSeq++
	}

	// 水位**在上传成功之后**才推进。
	// 顺序反了会在上传失败时丢掉增量（水位说传过了，实际没传）。
	if conversationId != "" {
		if err := model.SaveTrainingDataWatermark(
			signal.UserID, conversationId,
			nextWatermark.UploadedCount, nextWatermark.PrefixFingerprint,
			nextTextWatermark.UploadedBytes, nextTextWatermark.PrefixHash,
			time.Now().Unix(),
		); err != nil {
			// 水位保存失败：数据已上传，但下次会重复上传这段增量。
			// 重复上传比丢数据可接受（采集侧宁可冗余）。
			common.SysError("training capture: save watermark failed: " + err.Error())
		}
	}

	return len(increment), uploadedBytes, nil
}

// isSingleRequestBlob 判定「只有一条拼接文本」的数据形态。
//
// 这是当前生产路径的**唯一**形态（见 hotpath.go）：请求路径拿不到
// 结构化消息，只能给出 `CombineText`。判定成真时按文本前缀做增量。
//
// 判定刻意严格（恰好 1 条 且 role=="request"）：将来若真的加上了
// 结构化消息，会自动走回逐条语义更精确的 `SelectIncrement`，
// 不需要改这里。
func isSingleRequestBlob(messages []Message) bool {
	return len(messages) == 1 && messages[0].Role == "request"
}

// encodeJSONL 把消息编码为 JSONL（每行一条 JSON）。
//
// 选 JSONL 而非单个 JSON 数组：**可追加、可流式读、单行损坏不影响其它行**。
// 训练数据集的常见格式（且 GPT/Claude 微调都接受 messages 数组的逐行形式）。
func encodeJSONL(messages []Message) []byte {
	var b strings.Builder
	for _, m := range messages {
		record := map[string]string{
			"role":    m.Role,
			"content": m.Content,
		}
		line, err := common.Marshal(record)
		if err != nil {
			// 单条序列化失败不应让整批失败；跳过并继续。
			continue
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// splitBySize 把 payload 按 maxBytes 切分成块。
//
// **按行切分**（而不是按字节硬切）：硬切会把一行 JSON 切成两半，
// 云盘上的分片将无法逐行解析，数据集直接损坏。
func splitBySize(payload []byte, maxBytes int) [][]byte {
	if len(payload) == 0 {
		return nil
	}
	if maxBytes <= 0 || len(payload) <= maxBytes {
		return [][]byte{payload}
	}

	var chunks [][]byte
	start := 0
	for start < len(payload) {
		end := start + maxBytes
		if end >= len(payload) {
			chunks = append(chunks, payload[start:])
			break
		}
		// 从 end 往回找最近的换行，保证不切断行。
		cut := end
		for cut > start && payload[cut-1] != '\n' {
			cut--
		}
		if cut == start {
			// 单行就超过 maxBytes：只能硬切（否则死循环）。
			// 这种情况不该出现（单条消息由上游请求体上限约束），
			// 但必须能前进，不能无限循环。
			cut = end
		}
		chunks = append(chunks, payload[start:cut])
		start = cut
	}
	return chunks
}

// ---------- 全局指纹集合（退化路径用） ----------

var globalSeenFingerprints = NewBoundedFingerprintSet(FingerprintSetDefaultCapacity)

// ---------- 令牌桶限速 ----------

// byteLimiter 简单的字节令牌桶。
//
// 目的：**保护用户带宽**。上行是全站共享的，采集打满会让所有人的请求变慢。
// 刻意做得简单（不做突发桶）：采集不需要突发能力，
// 平滑速率对用户请求最友好。
type byteLimiter struct {
	ratePerSec int64
	mu         sync.Mutex
	lastRefill time.Time
	available  float64

	// stop 限速等待的中断信号。
	//
	// [修复防御] 原实现直接读**包级变量** `workerStop`：那让
	// `wait` 的行为依赖全局状态 —— 测试里前一个用例关掉 workerStop 后，
	// 后续用例的限速等待会**立即返回**（表现为「该等 0.5s 却等了 0s」，
	// 实测在整套测试里稳定复现）。把信号作为实例字段注入，
	// 限速器就只依赖自己的生命周期。
	stop <-chan struct{}
}

func newByteLimiter(ratePerSec int64) *byteLimiter {
	if ratePerSec <= 0 {
		ratePerSec = uploadRateLimitBytesPerSec
	}
	return &byteLimiter{
		ratePerSec: ratePerSec,
		lastRefill: time.Now(),
		// 初始给一个桶容量（1 秒的量），让首次上传不必等待。
		available: float64(ratePerSec),
	}
}

// NewByteLimiter 构造限速器，并绑定一个停止信号
// （worker 传入 `workerStop`；测试传 nil 即「只受 ctx 与计时器控制」）。
func NewByteLimiter(ratePerSec int64, stop <-chan struct{}) *byteLimiter {
	limiter := newByteLimiter(ratePerSec)
	limiter.stop = stop
	return limiter
}

// wait 消耗 n 字节的额度，不足时按速率等待。
func (l *byteLimiter) wait(ctx context.Context, n int64) {
	if l == nil || l.ratePerSec <= 0 || n <= 0 {
		return
	}
	l.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.available += elapsed * float64(l.ratePerSec)
	// 桶容量上限 = 1 秒的量（不做长时间积累导致的突发）。
	if maxBurst := float64(l.ratePerSec); l.available > maxBurst {
		l.available = maxBurst
	}
	l.lastRefill = now
	l.available -= float64(n)
	deficit := -l.available
	l.mu.Unlock()

	if deficit <= 0 {
		return
	}
	waitSecs := deficit / float64(l.ratePerSec)
	timer := time.NewTimer(time.Duration(waitSecs * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	case <-l.stop:
	}
}

// CleanupStaleWatermarks 清理过期的水位记录（防表无界增长）。
func CleanupStaleWatermarks() (int64, error) {
	cutoff := time.Now().Add(-watermarkCleanupAge).Unix()
	return model.DeleteStaleTrainingDataWatermarks(cutoff)
}

// cleanupOrphanParts 尽力删除本批已写入但**最终未提交**的分片。
//
// 为什么需要它：上传是逐块的，中途失败会留下「部分的这一批」。
// 水位不推进（正确：不丢数据），但孤儿分片会留在云盘上 ——
// 下轮重试会把整段增量**再写一遍**为更大的序号，
// 于是数据集里既有重复内容又有永远不该存在的残片。
//
// 语义边界（诚实说明）：
//   - 这是**补偿**（best-effort），不是事务。删除失败只记录日志，
//     不改变「本次上传失败」这个结论 —— 返回错误给上层，水位照样不推进。
//   - 单实例前提下安全：只删**本批自己刚写**的那几个名字，不会碰历史分片。
//     多实例会重新引入竞态（见启动日志里的多实例告警）。
func cleanupOrphanParts(ctx context.Context, sink TrainingDataSink, written []string) {
	if len(written) == 0 {
		return
	}
	deleter, ok := sink.(interface {
		DeleteFile(ctx context.Context, path string) error
	})
	if !ok {
		// 后端不支持删除（如未来某个只写不删的实现）：
		// 明确记录，而不是假装清理过。
		common.SysError(fmt.Sprintf(
			"training capture: %d orphan part(s) left on the sink (backend does not support delete): %v",
			len(written), written))
		return
	}
	for _, name := range written {
		if err := deleter.DeleteFile(ctx, name); err != nil {
			common.SysError(fmt.Sprintf(
				"training capture: failed to clean up orphan part %s: %s", name, err.Error()))
		}
	}
}
