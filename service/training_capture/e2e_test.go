package training_capture

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/relaykit/types"
)

// 端到端链路测试：**证明整条链真的接通**，而不是各段单独成立。
//
// 本项目踩过「每个模块都测过、库全绿，但生产上一个都没被调用」的坑
// （三个模块的库测试全绿，生产零调用）。那个坑的成因是：
// 单元测试直接调用模块函数，**绕过了「谁在生产里调用它」这一环**。
//
// 所以这里刻意**不直接调用 processSignal**，而是从「请求路径会调用的
// 唯一入口」开始：BuildSignal → MaybeCapture → 队列 → worker → sink。
// 一旦其中任何一环没接上（比如 worker 没启动、快速开关没同步），
// 这个测试就会失败。

// runWorkerOnce 启动真实 worker 并等待它把队列消费到指定条数。
//
// 用真实的 StartWorker 而不是手动调用 processSignal —— 后者会让
// 「worker 循环本身有 bug」（例如忘了从队列取、取到就 continue）
// 完全逃过测试。
func runWorkerOnce(t *testing.T, sink TrainingDataSink, expectSignals int) {
	t.Helper()
	restoreSink := SetSinkForTest(sink)
	t.Cleanup(restoreSink)

	// 用独立队列，避免与其它用例共享全局队列造成串扰。
	originalQueue := captureQueue
	captureQueue = NewCaptureQueue(64)
	t.Cleanup(func() { captureQueue = originalQueue })

	restoreGateway := SetGatewayEnabledForTest(true)
	t.Cleanup(restoreGateway)

	// 用真实 worker：它的循环会从 captureQueue 取信号并处理。
	workerOnce = sync.Once{}
	workerStop = make(chan struct{})
	done := make(chan struct{})
	go func() {
		runWorker()
		close(done)
	}()
	t.Cleanup(func() {
		StopWorker()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("worker 未在 5s 内停止（StopWorker 可能失效）")
		}
	})

	// 等待 worker 把队列消费完。
	deadline := time.After(10 * time.Second)
	for {
		if captureQueue.Stats().Len == 0 {
			// 队列空不等于处理完（worker 可能正在上传）。
			// 再给一个短窗口让在途的那条完成。
			time.Sleep(300 * time.Millisecond)
			if captureQueue.Stats().Len == 0 {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("worker 未在 10s 内消费完 %d 条信号（队列剩余 %d）",
				expectSignals, captureQueue.Stats().Len)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestEndToEndRelayToCloudDrive 是最重要的一条用例：
// 「一次成功的请求 → 云盘上出现一个分片文件」。
//
// 如果这条不通过，说明功能**在生产里不会工作**，无论各模块测试多绿。
func TestEndToEndRelayToCloudDrive(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	sink := newMemSink()
	runWorkerOnce(t, sink, 1)

	const userId = 9001
	grantForTest(t, userId)

	// 1) 模拟 controller 的调用：用真实 meta（含 CombineText）。
	meta := &types.TokenCountMeta{
		CombineText:   "user\n帮我写一个 Go 的 HTTP server\nassistant\n用 net/http 即可",
		MessagesCount: 2,
	}
	signal := BuildSignal(CaptureInput{
		UserId: userId, Username: "e2e-user", Model: "gpt-4o",
		ConversationID: "e2e-conv-1",
		Meta:           meta,
		CreatedAt:      1791937800,
	})
	if signal == nil {
		t.Fatal("BuildSignal 返回 nil —— 热路径未启用（快速开关与总开关/云盘不同步）")
	}

	// 2) 走真实入口投递。
	if !MaybeCapture(*signal) {
		t.Fatal("MaybeCapture 拒绝入队 —— 授权判定或队列有问题")
	}

	// 3) 等 worker 处理完。
	if !waitFor(t, 10*time.Second, func() bool { return sink.writeCallsCount() > 0 }) {
		t.Fatal("worker 未在 10s 内产生任何上传 —— 队列与 worker 之间没接通（死代码）")
	}

	// 4) 断言云盘上的产物：目录结构 + 内容。
	files := sink.snapshot()
	if len(files) == 0 {
		t.Fatal("云盘上没有任何文件")
	}
	var found bool
	for path, data := range files {
		const wantPrefix = "2026-10-14/e2e-user-" + "9001/gpt-4o/"
		if strings.HasPrefix(path, wantPrefix) {
			found = true
			if !strings.Contains(string(data), "帮我写一个 Go 的 HTTP server") {
				t.Fatalf("分片内容不含原文：%q", data)
			}
		}
	}
	if !found {
		t.Fatalf("云盘上没找到期望的三级目录产物，实际：%v", keysOf(files))
	}
}

// TestEndToEndSecondRequestUploadsOnlyIncrement 验证生产语义：
// 同一会话的第二次请求只上传增量。
//
// 这条在「真实 worker + 真实 DB 水位」下验证，而不是只测纯函数 ——
// 水位在 worker 里读、在上传成功后写，两处的接线错了单元测试看不出来。
//
// 注意数据形态（见 hotpath.go 的诚实边界）：请求路径只能拿到
// `CombineText` 这个**拼接头**，因此每次请求产生 **1 条** `role="request"`
// 消息，而不是逐轮的结构化消息。所以水位按「请求次数」推进。
func TestEndToEndSecondRequestUploadsOnlyIncrement(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	sink := newMemSink()
	runWorkerOnce(t, sink, 2)

	const userId = 9002
	grantForTest(t, userId)

	// 第一次：完整历史。
	meta1 := &types.TokenCountMeta{CombineText: "第一轮问题\n第一轮回答", MessagesCount: 2}
	sig1 := BuildSignal(CaptureInput{
		UserId: userId, Username: "inc", Model: "m", ConversationID: "e2e-conv-inc",
		Meta: meta1, CreatedAt: 1791937800,
	})
	if sig1 == nil || !MaybeCapture(*sig1) {
		t.Fatal("第一次请求未入队")
	}
	if !waitFor(t, 10*time.Second, func() bool { return sink.writeCallsCount() > 0 }) {
		t.Fatal("第一次上传未发生")
	}
	linesAfterFirst := sink.totalLines()
	if linesAfterFirst != 1 {
		t.Fatalf("首次应上传 1 条 request 记录，实际 %d", linesAfterFirst)
	}

	// 水位应记录**已上传的文本字节数**。
	// 数据形态是「单块拼接文本」（见 text_increment.go），
	// 所以增量水位在**文本前缀**上，不是在消息条数上。
	const firstText = "第一轮问题\n第一轮回答"
	wm1, err := loadWatermarkForTest(userId, "e2e-conv-inc")
	if err != nil {
		t.Fatalf("读水位：%v", err)
	}
	if wm1 == nil {
		t.Fatal("首次上传后应写入水位")
	}
	if wm1.RequestTextBytes != len(firstText) {
		t.Fatalf("首次后文本水位应为 %d 字节，实际 %d", len(firstText), wm1.RequestTextBytes)
	}
	if wm1.RequestTextHash == "" {
		t.Fatal("必须记录前缀指纹，否则无法察觉历史被编辑")
	}

	// 第二次：拼接串**前缀不变** + 追加新一轮。
	// 这是「长期会话」的真实形态：历史原封不动，只有尾部变长。
	const secondText = "第一轮问题\n第一轮回答\n第二轮问题\n第二轮回答"
	meta2 := &types.TokenCountMeta{CombineText: secondText, MessagesCount: 4}
	sig2 := BuildSignal(CaptureInput{
		UserId: userId, Username: "inc", Model: "m", ConversationID: "e2e-conv-inc",
		Meta: meta2, CreatedAt: 1791937800,
	})
	if sig2 == nil || !MaybeCapture(*sig2) {
		t.Fatal("第二次请求未入队")
	}
	// 等**水位**推进，而不是等行数增加：
	// worker 是「先 WritePart，后 SaveWatermark」，两者之间有窗口。
	// 只等行数会在窗口内就返回，让后面的水位断言读到旧值（-race 下实测复现）。
	if !waitFor(t, 10*time.Second, func() bool {
		wm, err := loadWatermarkForTest(userId, "e2e-conv-inc")
		return err == nil && wm != nil && wm.RequestTextBytes == len(secondText)
	}) {
		t.Fatal("第二次请求的水位未在 10s 内推进到预期值 —— 增量链路断了")
	}
	if sink.totalLines() <= linesAfterFirst {
		t.Fatal("第二次请求未产生新的上传")
	}

	// **核心断言**：第二次只上传了追加的那一段，不是整段重传。
	// 这是用户点名要求的「长期会话很多 token 都一样，要识别增量」。
	var uploadedSuffix string
	for _, data := range sink.snapshot() {
		content := string(data)
		if strings.Contains(content, "第二轮") {
			uploadedSuffix = content
		}
	}
	if uploadedSuffix == "" {
		t.Fatal("第二次的增量内容没找到")
	}
	if strings.Contains(uploadedSuffix, "第一轮") {
		t.Fatalf("第二次上传里包含**第一轮的历史**（说明在整段重传，增量失效）：%q", uploadedSuffix)
	}
	if !strings.Contains(uploadedSuffix, "第二轮问题") {
		t.Fatalf("增量内容不含新增的轮次：%q", uploadedSuffix)
	}

	wm2, err := loadWatermarkForTest(userId, "e2e-conv-inc")
	if err != nil {
		t.Fatalf("读水位：%v", err)
	}
	if wm2 == nil {
		t.Fatal("水位未写入 —— 下次会整段重传")
	}
	if wm2.RequestTextBytes != len(secondText) {
		t.Fatalf("水位应推进到 %d 字节，实际 %d", len(secondText), wm2.RequestTextBytes)
	}
}

// TestEndToEndUnauthorizedUserIsNeverCollected 是**合规底线**用例。
//
// 未授权用户的数据绝不能出现在云盘上。这条必须端到端验证：
// 授权判定发生在 MaybeCapture（热路径），而写入发生在 worker ——
// 两处都必须生效，中间被绕过就是隐私事故。
func TestEndToEndUnauthorizedUserIsNeverCollected(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	sink := newMemSink()
	runWorkerOnce(t, sink, 0)

	const userId = 9003 // **刻意不调用 grantForTest**
	ResetConsentCacheForTest()

	signal := BuildSignal(CaptureInput{
		UserId: userId, Username: "no-consent", Model: "m",
		Meta: &types.TokenCountMeta{CombineText: "未授权用户的私密对话"}, CreatedAt: 1791937800,
	})
	if signal == nil {
		t.Fatal("BuildSignal 不应关心授权（那是 MaybeCapture 的职责）")
	}
	if MaybeCapture(*signal) {
		t.Fatal("未授权用户的信号被入队了 —— 隐私事故")
	}

	// 给 worker 充分时间：**即使队列里意外有信号，也不该写任何东西**。
	time.Sleep(1 * time.Second)
	if got := sink.writeCallsCount(); got != 0 {
		t.Fatalf("未授权用户的数据被上传了（%d 次写入）—— 严重隐私事故", got)
	}
	if lines := sink.totalLines(); lines != 0 {
		t.Fatalf("云盘上出现了未授权用户的数据（%d 行）", lines)
	}
}

// TestEndToEndSkippedWhenGatewayDisabled 验证总开关关闭后不再采集。
func TestEndToEndSkippedWhenGatewayDisabled(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	sink := newMemSink()
	runWorkerOnce(t, sink, 0)

	const userId = 9004
	grantForTest(t, userId)
	SetGatewayEnabled(false) // 管理员关闭
	t.Cleanup(func() { SetGatewayEnabled(true) })

	signal := BuildSignal(CaptureInput{
		UserId: userId, Username: "off", Model: "m",
		Meta: &types.TokenCountMeta{CombineText: "关闭后不该被采集"}, CreatedAt: 1791937800,
	})
	if signal != nil {
		t.Fatal("总开关关闭后 BuildSignal 不该产出信号")
	}

	time.Sleep(500 * time.Millisecond)
	if sink.writeCallsCount() != 0 {
		t.Fatalf("总开关关闭后仍有上传（%d 次）", sink.writeCallsCount())
	}
}

// waitFor 轮询等待条件成立。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

var _ = context.Background
