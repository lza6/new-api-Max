package service

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
)

// T7 中继一致性自检的**请求级收口**。
//
// relay_audit.go 提供纯函数（Audit* 系列）；本文件把它们接到真实请求生命周期上：
//   - CollectRelayAudit 在响应处理过程中把发现追加到 gin.Context（线程安全）；
//   - FlushRelayAudit 在请求结束、写日志前把收集到的发现落到日志 admin_info，
//     同时计入 /metrics（RecordRelayAudit）。
//
// 开关关闭（RELAY_AUDIT_ENABLED 未开）时两者都是 no-op，且**不分配**任何内存，
// 保证零行为变化与零开销。

// maxRelayAuditFindingsPerRequest 单请求保留的发现上限，防止异常上游刷爆日志。
const maxRelayAuditFindingsPerRequest = 20

// relayAuditSink 由 gin.Context 持有的可变收集器。gin.Context 本身不是并发安全的，
// 但流式路径的 scanner goroutine 会与主 goroutine 并发调用 CollectRelayAudit，
// 故收集器内部自带互斥。
type relayAuditSink struct {
	mu       sync.Mutex
	findings []RelayAuditFinding
	seen     map[string]struct{}
	dropped  int

	// usageState 跟踪本次流式的 usage 单调性（仅流式路径使用）。
	usageState *usageMonotonicState
	// fingerprintChecked 标记是否已做过模型指纹校验（只需一次）。
	fingerprintChecked bool
}

// CollectRelayAudit 追加一组自检发现到当前请求。开关关闭或开关为 nil 时直接返回。
// 同一 check+detail 只保留一条（去重），避免流式每帧重复写同一条。
func CollectRelayAudit(c *gin.Context, findings ...RelayAuditFinding) {
	if c == nil || len(findings) == 0 || !RelayAuditEnabled() {
		return
	}
	sink := relayAuditSinkFrom(c)
	if sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	appendFindingsLocked(sink, findings)
}

// relayAuditSinkFrom 取得（必要时创建）当前请求的收集器。
func relayAuditSinkFrom(c *gin.Context) *relayAuditSink {
	if v, ok := common.GetContextKeyType[*relayAuditSink](c, constant.ContextKeyRelayAuditFindings); ok {
		return v
	}
	sink := &relayAuditSink{}
	common.SetContextKey(c, constant.ContextKeyRelayAuditFindings, sink)
	return sink
}

// RelayAuditFindings 返回当前请求已收集的发现（只读快照）。供日志与测试读取。
func RelayAuditFindings(c *gin.Context) []RelayAuditFinding {
	if c == nil {
		return nil
	}
	sink, ok := common.GetContextKeyType[*relayAuditSink](c, constant.ContextKeyRelayAuditFindings)
	if !ok || sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.findings) == 0 {
		return nil
	}
	out := make([]RelayAuditFinding, len(sink.findings))
	copy(out, sink.findings)
	return out
}

// RelayAuditDroppedCount 返回因超出单请求上限而被丢弃的发现数（0 表示未丢弃）。
func RelayAuditDroppedCount(c *gin.Context) int {
	if c == nil {
		return 0
	}
	sink, ok := common.GetContextKeyType[*relayAuditSink](c, constant.ContextKeyRelayAuditFindings)
	if !ok || sink == nil {
		return 0
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.dropped
}

// FlushRelayAudit 把本次请求的自检发现落到日志（admin_info.relay_audit）并计入指标。
// 由日志生成路径在写完 other 之前调用一次。无发现时为 no-op（不写空字段）。
//
// 放置于 admin_info：自检细节含上游模型名/错误片段，属运维排障信息，不应向普通
// 用户暴露；模型可见性逻辑会自动对非管理员剥离 admin_info。
func FlushRelayAudit(c *gin.Context, setAdmin func(key string, value any) bool) {
	if c == nil || setAdmin == nil || !RelayAuditEnabled() {
		return
	}
	findings := RelayAuditFindings(c)
	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		RecordRelayAudit(f)
	}
	setAdmin("relay_audit", findings)
	if dropped := RelayAuditDroppedCount(c); dropped > 0 {
		setAdmin("relay_audit_dropped", dropped)
	}
}

// AuditStreamFrame 对一帧流式 SSE data（已解出 JSON 字符串）做自检：
// SSE 顶层键白名单 + usage 单调性。由流式扫描路径逐帧调用。
//
// 只用一次 gjson 解析（不反序列化整帧），开关关闭时首行返回，零开销。
func AuditStreamFrame(c *gin.Context, frameJSON string) {
	if c == nil || frameJSON == "" || !RelayAuditEnabled() {
		return
	}
	sink := relayAuditSinkFrom(c)
	if sink == nil {
		return
	}
	if !gjson.Valid(frameJSON) {
		// 非 JSON 帧（[DONE] 等）不参与检查。
		return
	}
	root := gjson.Parse(frameJSON)
	if !root.IsObject() {
		return
	}

	// 1) SSE 顶层键白名单。
	keys := make([]string, 0, 8)
	root.ForEach(func(key, _ gjson.Result) bool {
		keys = append(keys, key.String())
		return true
	})

	// 2) usage 单调性（仅当本帧带 usage 时推进状态）。
	usage := root.Get("usage")

	sink.mu.Lock()
	defer sink.mu.Unlock()

	appendFindingsLocked(sink, AuditSSEWhitelist(keys))

	if usage.Exists() && usage.IsObject() {
		if sink.usageState == nil {
			sink.usageState = NewUsageMonotonicState()
		}
		appendFindingsLocked(sink, AuditUsageMonotonic(
			sink.usageState,
			int(usage.Get("prompt_tokens").Int()),
			int(usage.Get("completion_tokens").Int()),
			int(usage.Get("total_tokens").Int()),
		))
	}
}

// AuditModelFingerprintOnce 对上游回显的模型名做一次同族校验（每个请求只查一次）。
func AuditModelFingerprintOnce(c *gin.Context, requestedModel, upstreamModel string) {
	if c == nil || requestedModel == "" || upstreamModel == "" || !RelayAuditEnabled() {
		return
	}
	sink := relayAuditSinkFrom(c)
	if sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.fingerprintChecked {
		return
	}
	sink.fingerprintChecked = true
	appendFindingsLocked(sink, AuditModelFingerprint(requestedModel, upstreamModel))
}

// AuditUpstreamError 对上游/网关错误文本做泄漏检查（密钥/Bearer/堆栈）。
// 由错误处理路径调用；无论开关是否开启都不修改错误本身，只做记录。
func AuditUpstreamError(c *gin.Context, errText string) {
	if c == nil || errText == "" || !RelayAuditEnabled() {
		return
	}
	sink := relayAuditSinkFrom(c)
	if sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	appendFindingsLocked(sink, AuditErrorLeak(errText))
}

// appendFindingsLocked 在**已持 sink.mu** 的前提下追加发现（去重 + 上限）。
func appendFindingsLocked(sink *relayAuditSink, findings []RelayAuditFinding) {
	if sink == nil || len(findings) == 0 {
		return
	}
	if sink.seen == nil {
		sink.seen = make(map[string]struct{}, len(findings))
	}
	for _, f := range findings {
		if f.Check == "" {
			continue
		}
		key := f.Check + "|" + f.Detail
		if _, ok := sink.seen[key]; ok {
			continue
		}
		if len(sink.findings) >= maxRelayAuditFindingsPerRequest {
			sink.dropped++
			continue
		}
		sink.seen[key] = struct{}{}
		sink.findings = append(sink.findings, f)
	}
}
