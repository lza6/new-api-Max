package training_capture

import (
	"context"
	"errors"
	"fmt"
	"os"
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

	// workerMaxSignalRetries 单条信号最多回队重试几次。
	//
	// 为什么必须有上限：云盘长期不可用时，无限重试会占满资源、刷日志，
	// 把后来的正常信号挤掉。取 3：配合指数退避（基 2s，首次 4s，
	// 依次 8s / 16s，三次合计约 28s），足够扛过瞬时故障
	// （429、token 刷新、网络抖动），又不至于长期滞留。
	workerMaxSignalRetries = 3

	// MaxPartBytes 单个分片的目标大小。
	//
	// 必须 < 后端单次写入上限（OneDrive 是 4MB，见
	// `PartUploadLimiter`/`EffectiveMaxWritePartBytes`），否则会走上传会话
	// （另一套复杂逻辑）。取 2MB：既减少文件数，又留足余量。
	// **这是「文件不会无限增大」的实现**（用户明确要求轮转）。
	//
	// 注意：这是**目标**大小；真实生效的原始上限是
	// `EffectiveMaxPartRawBytes()`（按内存预算自适应），
	// 压缩后若仍超过后端上限会由 `writeAggregateParts` 再切一刀。
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

	// TotalSkippedLease 因其他实例持有该用户采集租约而跳过的次数（多实例协调）。
	TotalSkippedLease atomic.Int64

	// TotalRetried 因上传失败而**回队重试**的次数。
	//
	// 与 TotalUploadErrors 的区别：后者是「失败次数」，前者是「失败后成功回队」。
	// 两者一起看才能区分「偶发抖动自愈」与「一直失败且队列在堆积」。
	TotalRetried atomic.Int64

	// usersToday 今日去重用户集合（有界，只存今日）。
	usersTodayMu sync.Mutex
	usersToday   map[int]struct{}
}

var captureStats = &CaptureStats{}

// captureLeaseOwner 标识本进程，用于多实例采集租约（host:pid）。
var captureLeaseOwner = func() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}()

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
	SkippedLease      int64  `json:"skipped_lease"`
	// Retried 失败后成功回队的次数（配合 upload_errors 判断是自愈还是持续故障）。
	Retried  int64 `json:"retried"`
	QueueLen int   `json:"queue_len"`
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
		SkippedLease:      captureStats.TotalSkippedLease.Load(),
		Retried:           captureStats.TotalRetried.Load(),
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

// worker 生命周期状态。
//
// [修复防御] **这组状态的存在是为了修一个会让采集永久停摆的缺陷。**
//
// 原实现：`StartWorker` 用 `workerOnce.Do`（只生效一次），
// `StopWorker` 关掉 `workerStop`。于是「管理员关开关 → 再打开」时：
//   - 第二次 `StartWorker` 是 **no-op**（once 已消费）；
//   - `workerStop` 已是**已关闭**的 channel → 新起的循环会立刻退出
//     （即使真起了）。
//
// 结果：**队列继续被灌满、dropped 计数上涨，而管理端仍显示 enabled** ——
// 典型的「看起来在工作其实没工作」。
//
// 更糟的是 worker 顶层 panic 恢复的注释当时写着「由调用方 StartWorker
// 负责重启」—— 而那个重启机制本身就是坏的。注释在宣称一个不存在的保证。
//
// 修法：用 `workerRunning` 标志 + 每次 `StartWorker` **新建** stop channel，
// 让启停可以任意次往返；并用同一把锁保证不会起两个 worker。
var (
	workerMu      sync.Mutex
	workerStop    chan struct{}
	workerDone    chan struct{}
	workerRunning bool
)

// workerStopWaitTimeout 停止/重启时等待旧 worker 真正退出的上限。
const workerStopWaitTimeout = 5 * time.Second

// StartWorker 启动采集 worker（**可重复调用**：已运行则 no-op；已停止则重启）。
//
// ## 为什么 stop channel 必须**作为参数**传给 goroutine
//
// 原实现让 `runWorker` 直接读包级 `workerStop`。于是：
//  1. 数据竞争 —— `StartWorker` 在锁内写它，而 worker goroutine 在循环里
//     无同步地读（`go test -race` 直接报 DATA RACE）；
//  2. **僵尸 worker** —— `StopWorker` 关闭 C1 后，`StartWorker` 立刻把
//     全局变量换成 C2；尚未被调度的旧 goroutine 读到 C2（未关闭）→ 永不退出
//     → **两个 goroutine 同时消费同一队列**。两条同会话信号并发处理时，
//     各自读水位→上传→写水位，后写覆盖前写，可能重复或丢失增量。
//
// 现在每次启动把 stop/done 作为**局部值**捕获，循环只认自己那一个。
//
// 返回是否**实际启动了**新 worker（供调用方记日志/断言）。
func StartWorker() bool {
	workerMu.Lock()
	if workerRunning {
		workerMu.Unlock()
		return false
	}
	prevDone := workerDone
	workerMu.Unlock()

	// 等旧 worker 真正退出，避免「旧消费者还没走、新消费者已来」的窗口。
	if prevDone != nil {
		select {
		case <-prevDone:
		case <-time.After(workerStopWaitTimeout):
			// 不假装已停：如实记日志。仍然启动（否则功能不可用），
			// 但至少让这个异常可见。
			common.SysError("training capture: previous worker did not exit in time; starting a new one anyway")
		}
	}

	workerMu.Lock()
	defer workerMu.Unlock()
	// 上面的等待释放过锁，可能有并发的 StartWorker 抢先启动。
	if workerRunning {
		return false
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	workerStop = stop
	workerDone = done
	workerRunning = true
	go runWorker(stop, done)
	return true
}

// StopWorker 停止采集 worker（可重复调用）。
//
// 通过关闭 stop channel 让 worker 的 `Receive`/限速等待被唤醒并退出 ——
// 不丢已入队的数据（ReceiveWithStop 在退出前会把缓冲取完）。
//
// **会等待 goroutine 真正退出**（有界）：只把标志置 false 就返回，
// 会让紧接着的 `StartWorker` 在旧消费者仍在跑时启动新消费者 ——
// 那就是两个 worker 并发读写同一个会话水位。
//
// 返回是否**实际停止了**一个在跑的 worker。
func StopWorker() bool {
	workerMu.Lock()
	if !workerRunning {
		workerMu.Unlock()
		return false
	}
	workerRunning = false
	stop := workerStop
	done := workerDone
	workerMu.Unlock()

	if stop != nil {
		close(stop)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(workerStopWaitTimeout):
			common.SysError("training capture: worker did not exit within timeout after stop request")
		}
	}
	return true
}

// WorkerRunning 报告 worker 是否在运行（供指标与自检）。
//
// 这是「看起来开着其实没在采」的**可观测信号**：
// `training_capture_worker_running` 为 0 而总开关为开 → 有问题。
func WorkerRunning() bool {
	workerMu.Lock()
	defer workerMu.Unlock()
	return workerRunning
}

// runWorker 主循环。
//
// stop / done 都是**本次启动的局部值**（不读包级变量）——
// 见 `StartWorker` 关于数据竞争与僵尸 worker 的说明。
func runWorker(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
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
			// 兜底：即使循环**之外**的地方 panic（如 limiter 构造），
			// 也只让 worker 退出，不让整个网关进程倒下。
			// 退出后由 `WorkerRunning()` 暴露为「已停止」，
			// 管理端重启开关即可恢复（StartWorker 现在可重复调用）。
			workerMu.Lock()
			workerRunning = false
			workerMu.Unlock()
			captureStats.TotalUploadErrors.Add(1)
			common.SysError(fmt.Sprintf(
				"training capture: worker exited after panic (gateway unaffected; "+
					"re-enable the switch to restart): %v", r))
		}
	}()

	// 把**本次启动的** stop 绑进限速器：停机时限速等待也要被打断，
	// 否则一个正在等 2 分钟的限速会让 StopWorker 卡住。
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, stop)
	consecutiveErrors := 0

	// pendingRetries 暂存**本 worker 内**待重试的信号。
	//
	// [修复防御] 不能把失败信号放回队列尾部：同一会话的新信号可能先被处理
	// 并推进水位，旧信号再重试时前缀校验失败 → 整段重传（重复数据）。
	// 放在这里并**优先于队列**处理，保证同一 worker 内的顺序：
	//   旧信号重试 → 新信号。
	// 单 worker 同时只有一条信号在飞，所以这里实际最多 1 条，内存有界。
	var pendingRetries []CaptureSignal
	defer func() {
		// 停机时把未完成的重试尽量放回队列（尽力而为；队列满则计数丢弃）。
		for _, sig := range pendingRetries {
			if captureQueue != nil && captureQueue.TryEnqueue(sig) {
				continue
			}
			captureStats.TotalDropped.Add(1)
		}
	}()

	for {
		select {
		case <-stop:
			return
		default:
		}

		// 重试优先：它们比队列里的新信号更旧，先处理可避免顺序倒置。
		var signal CaptureSignal
		var ok bool
		if len(pendingRetries) > 0 {
			signal, pendingRetries = pendingRetries[0], pendingRetries[1:]
			ok = true
		} else {
			// 把 stop 一并交给 Receive：否则 worker 阻塞在这里等数据时
			// StopWorker 无法唤醒它（两个是不同的 channel，见 ReceiveWithStop）。
			signal, ok = captureQueue.ReceiveWithStop(stop)
		}
		if !ok {
			// 队列关闭（停机）或暂无数据。Receive 是阻塞的，
			// 不会走到这里空转；保险起见仍睡一下。
			select {
			case <-stop:
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

		// 多实例协调：同一用户同一时刻只允许一个实例上传，避免水位互相覆盖。
		leaseOK, leaseErr := model.AcquireTrainingCaptureLease(signal.UserID, captureLeaseOwner, 5*time.Minute)
		if leaseErr != nil {
			captureStats.TotalSkippedLease.Add(1)
			common.SysError("training capture: acquire lease failed for user " + fmt.Sprint(signal.UserID) + ": " + leaseErr.Error())
			continue
		}
		if !leaseOK {
			captureStats.TotalSkippedLease.Add(1)
			continue
		}

		uploaded, uploadedBytes, err := processSignalSafely(context.Background(), signal, limiter)
		if releaseErr := model.ReleaseTrainingCaptureLease(signal.UserID, captureLeaseOwner); releaseErr != nil {
			common.SysError("training capture: release lease failed for user " + fmt.Sprint(signal.UserID) + ": " + releaseErr.Error())
		}
		if err != nil {
			captureStats.TotalUploadErrors.Add(1)
			consecutiveErrors++
			backoff := workerErrorBackoff * time.Duration(1<<min(consecutiveErrors, 6))
			if backoff > workerMaxBackoff {
				backoff = workerMaxBackoff
			}
			logWorkerError(signal, err, consecutiveErrors)

			// **重试**：信号已被 `ReceiveWithStop` 取走，不重试就永久丢。
			//
			// [修复防御] 多账号 fail-closed（无法判定目录归属就拒绝写入）
			// 之后，这条变得**必需**：否则「宁可不写」会直接变成「数据丢了」。
			// 对已经结束的会话尤其致命 —— 没有下一次请求来重放这段增量。
			//
			// 放进 `pendingRetries`（下一轮**优先**处理），而不是回队列尾部：
			// 否则更新的同会话信号可能先推进水位，旧信号重试时整段重传。
			if signal.Attempt < workerMaxSignalRetries {
				signal.Attempt++
				pendingRetries = append(pendingRetries, signal)
				captureStats.TotalRetried.Add(1)
			} else {
				common.SysError(fmt.Sprintf(
					"training capture: giving up after %d retries (%d processing attempts), this increment is lost (user=%d model=%s)",
					signal.Attempt, signal.Attempt+1, signal.UserID, signal.Model))
			}

			if consecutiveErrors >= workerMaxConsecutiveErrors {
				// 连续失败 → 长退避（云盘可能长期不可用；
				// 拼命重试只会刷日志、占资源）。
				backoff = workerMaxBackoff
			}
			select {
			case <-stop:
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

	// 组装消息序列。
	//
	// [修复防御] 助手回复**按增量路径区别对待**，不能一刀切：
	//
	//   - **文本前缀路径**（当前生产形态：单条拼接文本）：
	//     回复**不并入** messages —— 并进去会让消息数变成 2、
	//     `isSingleRequestBlob` 判假、增量从「文本前缀」退化成
	//     「消息级」→ 每轮全量重传（去重静默失效）。
	//     回复在增量算出**之后**追加为一条记录。
	//   - **消息级路径**（将来的结构化消息）：回复本身就是一条真实消息，
	//     必须并入 messages 才能被水位正确追踪。
	reply := strings.TrimSpace(signal.ResponseText)
	messages := append([]Message(nil), signal.Messages...)
	useTextIncrement := isSingleRequestBlob(messages)
	if !useTextIncrement && reply != "" {
		messages = append(messages, Message{Role: "assistant", Content: reply})
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
		if useTextIncrement {
			// 超限截断的会话：新内容在**尾部**，而存储文本含「省略 N 字节」
			// 标记（N 每轮都变）—— 对整段做前缀比对会退化成每轮全量重传。
			//
			// 这修复了一个**静默丢数据**的缺陷：原实现对超限会话只保留头部，
			// 前缀恒定 → 增量后缀恒为空 → **尾部新内容永远不被采集**。
			//
			// ⚠️ **诚实边界（别把它读成「真正的尾部增量」）**：
			// `truncateForCapture` 取的尾部窗口会随文本增长而**平移**，
			// 所以 `tail(n+1)` 并不以 `tail(n)` 为前缀 → 前缀校验必然失败
			// → `SelectTextIncrement` 每轮返回**整段尾部窗口**（保守全量），
			// 而不是「只追加的新段」。
			//
			// 也就是说：对超限会话，每轮会上传约「半个上限」的重复尾部。
			// 这是**刻意的取舍** —— 数据不丢优先于带宽（采集不可逆，
			// 尾部是新内容所在处，漏掉就是永久丢失）。
			// 若要真正做到「只传新段」，需要按尾部窗口的**滚动偏移**建水位，
			// 那是独立设计，不在本函数的范围内。
			if signal.TailOnly != "" {
				textSuffix, nextTextWatermark = SelectTextIncrement(signal.TailOnly, currentText)
				if textSuffix != "" {
					increment = []Message{{
						Role: "request", Content: textSuffix,
						Kind: requestKindFor(textSuffix, signal.TailOnly),
					}}
				}
			} else {
				textSuffix, nextTextWatermark = SelectTextIncrement(messages[0].Content, currentText)
				if textSuffix != "" {
					increment = []Message{{
						Role: "request", Content: textSuffix,
						Kind: requestKindFor(textSuffix, messages[0].Content),
					}}
				}
			}
		} else {
			increment, nextWatermark = SelectIncrement(messages, current)
			// 消息级路径同样必须写 `kind`：否则导出声明为 v2 的分片里
			// 会出现「没有 kind 的 request 行」，消费方无法判断如何拼接
			// （见 KindRequestMessage 的说明）。
			prepareRequestRows(increment, KindRequestMessage)
		}
	} else {
		// 退化路径：拿不到会话 id 时按内容指纹全局去重。
		increment = GlobalDedup(messages, globalSeenFingerprints)
		prepareRequestRows(increment, KindRequestMessage)
	}

	if len(increment) == 0 {
		return 0, 0, nil
	}

	// 把**助手回复**追加到本批增量里（它是这次请求产出的、与新增请求内容配套）。
	//
	// 为什么放在这里而不是并进 Messages：
	// 并进 Messages 会让 `isSingleRequestBlob` 判假 → 增量从「文本前缀」
	// 退化到「消息级」→ 每轮全量重传（见 hotpath.go 的说明）。
	// 放在增量算出**之后**追加，既拿到回复又不影响增量语义。
	//
	// 规则：**只在请求侧有新增内容时才带回复**。若请求内容没变
	// （对话没推进），则什么都不传 —— 这与「增量」语义一致，
	// 不会把同一条回复反复上传。
	if useTextIncrement && reply != "" {
		increment = append(increment, Message{Role: "assistant", Content: reply})
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

		// 压缩率意外差时（已压缩内容 / 随机数据）压缩后仍可能超过**后端**
		// 单次写入上限：那时**再切一刀**。
		//
		// [修复防御] 关键：这一刀必须切在**原始字节**上，并各自重新 gzip。
		// 旧写法是 `splitBySize(blob, ...)` —— 直接切**压缩产物**，于是每个
		// 碎片都不是合法 gzip member（正是 ADR-003 记录的那类损坏）：
		// 预览按 magic 判定失败当明文显示二进制垃圾，分析端把碎片喂给模型。
		// 主路径修了、这个兜底分支没修，等于把同一个 bug 留在最难触发也
		// 最难排查的地方（只有高压缩比内容才会走进来）。
		//
		// 切原始字节则每个子块都能独立压缩成完整 member，可独立解压。
		// 递归深度天然为 1：切完后的子块远小于上限，不会再进入此分支。
		if limit := EffectiveMaxWritePartBytes(sink); len(blob) > limit {
			subChunks := resplitCompressedBetter(rawChunk, limit)
			common.SysLog(fmt.Sprintf(
				"training capture: part %d compresses poorly (%d -> %d bytes, limit %d), re-split into %d raw chunks",
				partSeq, len(rawChunk), len(blob), limit, len(subChunks)))

			for _, subRaw := range subChunks {
				subBlob, subErr := gzipCompress(subRaw)
				if subErr != nil {
					common.SysError("training capture: gzip failed on re-split, uploading plain: " + subErr.Error())
					subBlob = subRaw
				}
				subName := dirPath + "/" + PartFileName(partSeq)
				limiter.wait(ctx, int64(len(subBlob)))
				captureStats.InFlightBytes.Add(int64(len(subBlob)))
				_, writeErr := sink.WritePart(ctx, subName, subBlob)
				captureStats.InFlightBytes.Add(-int64(len(subBlob)))
				if writeErr != nil {
					cleanupOrphanParts(ctx, sink, written)
					return 0, 0, fmt.Errorf("write part %s: %w", subName, writeErr)
				}
				written = append(written, subName)
				uploadedBytes += int64(len(subBlob))
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
		// kind 只在有值时写出：assistant 行不带该字段，
		// 保持「回复行总是完整的」这个简单事实不被多余字段干扰。
		if m.Kind != "" {
			record[RequestKindField] = m.Kind
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

// resplitCompressedBetter 把一个「压缩后仍超后端上限」的原始块切成若干
// **原始**子块，使每块压缩后都 ≤ limit。返回的每个子块都可独立 gzip 成完整 member。
//
// ## 为什么这样切（而不是按压缩比估一个大小）
//
// 压缩比是**内容相关**的：同一段文本改成随机数据，比例能从 1/10 变成 1。
// 用「当前块的压缩比」去反推原始子块大小，是在赌「子块的压缩比与整块相同」——
// 这个假设没有依据，赌错就仍然是超限上传（WritePart 直接报错）。
//
// 这里改成**按实测收敛**：拿当前块的压缩比做初值，切完**逐个试压**，
// 任何仍超限的子块递归再切。因为「切小 → 压缩后更小」单调成立，
// 过程必然终止；且退出时每个子块都是**已验证**压缩后 ≤ limit 的，
// 不是估计值。
//
// ## 退出保证
//
// 每次递归至少切两块（splitBySize 在 len > maxBytes 时必产出 ≥2 块），
// 因此总量严格下降。兜底：若某轮因「单行超长」而没切动（切出 1 块且
// 长度不变），直接把子块大小减半，保证仍在前进。
func resplitCompressedBetter(raw []byte, limit int) [][]byte {
	if limit <= 0 || len(raw) == 0 {
		return [][]byte{raw}
	}
	// 初值：按当前块的实测压缩比反推，并保守取一半（因为这里就是
	// 「压缩不理想」的路径，实际比值很可能比整块差）。
	compressed, err := gzipCompress(raw)
	ratio := 1.0
	if err == nil && len(raw) > 0 {
		ratio = float64(len(compressed)) / float64(len(raw))
	}
	if ratio <= 0 {
		ratio = 1
	}
	subSize := int(float64(limit)/ratio) / 2
	if subSize < 1 {
		subSize = 1
	}
	return resplitRecursive(raw, limit, subSize, 0)
}

// resplitRecursive depth 仅用于兜底收缩，正常情况下第一层就收敛。
func resplitRecursive(raw []byte, limit, subSize, depth int) [][]byte {
	// 已经能放下 → 直接返回（这是唯一的成功出口）。
	if blob, err := gzipCompress(raw); err == nil && len(blob) <= limit {
		return [][]byte{raw}
	}
	chunks := splitBySize(raw, subSize)
	// 没切动（subSize ≥ 长度，或单行超长只能硬切回原块）→ 强制减半，保证前进。
	if len(chunks) == 1 && len(chunks[0]) == len(raw) {
		if subSize <= 1 || depth > 32 {
			// 已到极限：单字节块无法再小。返回原块，由上层 WritePart
			// 如实报错（宁可见错误，不可静默产出坏数据）。
			return [][]byte{raw}
		}
		return resplitRecursive(raw, limit, subSize/2, depth+1)
	}
	var out [][]byte
	for _, c := range chunks {
		out = append(out, resplitRecursive(c, limit, subSize, depth+1)...)
	}
	return out
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
			// 已经不存在的分片不需要清理（多账号下归属解析可能把它归到
			// 另一个账号，见 MultiAccountSink.DeleteFile）——这不是故障。
			if errors.Is(err, ErrFileNotFound) {
				continue
			}
			common.SysError(fmt.Sprintf(
				"training capture: failed to clean up orphan part %s: %s", name, err.Error()))
		}
	}
}

// requestKindFor 判定这条请求行是「完整快照」还是「增量后缀」。
//
// 判据：**增量与整段文本长度相同** → 它就是全量。
//
// 为什么会相等（三种真实情况，都必须标 full）：
//  1. 首次采集（水位为空）；
//  2. 历史被改动 → `SelectTextIncrement` 保守全量重传；
//  3. **水位被清理后重新开始**（`DeleteStaleTrainingDataWatermarks`
//     删掉了长期不活跃会话的水位，下次采集从零开始）。
//
// 消费方对 full 是「重置对话文本」、对 suffix 是「追加」——规则相反。
// 不打这个标，消费方**无法从数据本身**区分两者（第 3 种情况尤其隐蔽：
// 任意分片都可能以 full 开头），结果是无声的数据污染。
func requestKindFor(suffix, whole string) string {
	// 判据：**增量与整段文本一样长** → 它就是全量。
	//
	// 推导：`suffix = whole[prev.UploadedBytes:]`，所以「两者等长」
	// 当且仅当 `prev.UploadedBytes == 0`，也就是「这段文本此前一条都没传过」。
	//
	// 三种真实情况都落到这里，都必须标 full：
	//  1. 首次采集（水位为空）；
	//  2. 历史被改动 → SelectTextIncrement 保守全量重传；
	//  3. **水位被清理后重新开始**（DeleteStaleTrainingDataWatermarks
	//     删掉长期不活跃会话的水位，下次采集从零开始）——
	//     这种情况尤其隐蔽：**任意分片**都可能以 full 开头。
	//
	// 消费方对 full 是「重置对话文本」、对 suffix 是「追加」，规则相反。
	// 不打这个标，消费方无法从数据本身区分两者，结果是无声的数据污染。
	if whole != "" && len(suffix) == len(whole) {
		return KindRequestFull
	}
	return KindRequestSuffix
}

// markRequestKind 给一批**已产出的增量消息**里的请求行打上形态标记。
//
// 用于消息级路径（`SelectIncrement`）与无会话 id 的全局去重路径
// （`GlobalDedup`）：这两条路径产出的是结构化消息，既不是「对话快照」
// 也不是「文本后缀」，必须标记成 `KindRequestMessage`，
// 否则导出声明为 v2 的分片里会出现**没有 kind 的 request 行**。
//
// assistant 行不打标（回复总是完整的，见 `Message.Kind` 的说明）。
func prepareRequestRows(msgs []Message, kind string) {
	for i := range msgs {
		// 1) 角色归一：结构化消息路径的用户侧可能标成 `user`，
		//    而导出契约是 `role ∈ {request, assistant}` ——
		//    `user` 会被消费方按未知角色丢弃。语义一致，统一成 `request`。
		if msgs[i].Role == "user" {
			msgs[i].Role = "request"
		}
		// 2) 形态标记：它们是「若干条完整消息」，既非对话快照也非文本后缀。
		if msgs[i].Role == "request" && msgs[i].Kind == "" {
			msgs[i].Kind = kind
		}
	}
}

// processSignalSafely 在**单条信号**外包裹 panic 恢复。
//
// [修复防御] 为什么恢复要放在**每条信号**而不是整个循环外面：
//
// 循环外的 recover 只能让 worker **退出**（不让进程倒下），
// 但一条坏数据就会让采集**永久停摆**，直到有人手动重启开关。
// 恢复放在这里则：坏数据被丢弃并计入错误，**worker 继续处理下一条**。
// 「采到坏数据」与「不再采集」是两件严重程度完全不同的事。
//
// 注意：这不能替代循环外的兜底 recover（循环体其它地方、限速器等
// 仍可能 panic），两者是纵深防御的两层。
func processSignalSafely(ctx context.Context, signal CaptureSignal, limiter *byteLimiter) (uploaded int, uploadedBytes int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			captureStats.TotalUploadErrors.Add(1)
			common.SysError(fmt.Sprintf(
				"training capture: recovered from panic while processing one signal "+
					"(user=%d model=%s); skipping this record and continuing: %v",
				signal.UserID, signal.Model, r))
			uploaded, uploadedBytes, err = 0, 0, fmt.Errorf("panic while processing signal: %v", r)
		}
	}()
	return processSignal(ctx, signal, limiter)
}
