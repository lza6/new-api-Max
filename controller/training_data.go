package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/service/training_capture"
)

// 训练数据采集的 HTTP 接口（Spec-008 / T4.1 用户端 + T4.2 管理端）。
//
// ## 安全契约（每条都对应一个真实攻击面）
//
//  1. **用户 ID 只从会话取**（`c.GetInt("id")`），**绝不**从请求体/查询串取。
//     否则任何登录用户都能替别人授权或撤回（越权写）。
//  2. **管理端全部走 `AdminAuth()` 中间件**（在路由处声明），
//     统计里含全站采集规模，属运营数据。
//  3. **`available` 由服务端判定**（总开关 ∧ 云盘已配置），不信前端。
//     前端拿到 false 就不渲染开关（用户要求：管理员关了就不该显示）。
//  4. 写操作（授权/撤回/改总开关）走 `common.ApiSuccess`/`ApiError` 统一信封。

// ---------- 用户端（T4.1） ----------

// GetTrainingDataStatus 返回当前用户的采集授权状态。
//
// GET /api/user/training-data/status
func GetTrainingDataStatus(c *gin.Context) {
	// 只信会话里的用户 ID。
	userId := c.GetInt("id")
	status, err := training_capture.GetConsentStatus(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

// GrantTrainingDataConsent 记录用户同意采集。
//
// POST /api/user/training-data/grant
//
// 请求体可选 `{"policy_version":"v1"}`；**不传则用服务端当前版本**。
// 刻意不信任前端传来的版本号：若允许前端指定任意版本，用户可以
// 「同意一个不存在的旧版本」来绕过版本校验（那是合规缺口）。
func GrantTrainingDataConsent(c *gin.Context) {
	userId := c.GetInt("id")

	var body struct {
		PolicyVersion string `json:"policy_version"`
	}
	// 请求体解析失败不致命：本接口无必填字段。
	_ = c.ShouldBindJSON(&body)

	// 服务端权威版本：永远用当前版本，忽略前端传入的其它值。
	version := training_capture.CurrentPolicyVersion
	if v := strings.TrimSpace(body.PolicyVersion); v != "" && v != version {
		// 前端拿到的版本与服务端不一致 → 说明声明已更新，
		// 用户同意的是旧条款，**必须拒绝**并让前端刷新后重新确认。
		common.ApiErrorMsg(c, fmt.Sprintf(
			"policy version mismatch: server=%s requested=%s; refresh the disclaimer and try again",
			version, v))
		return
	}

	if err := training_capture.GrantConsent(userId, version); err != nil {
		common.ApiError(c, err)
		return
	}
	// 回读状态：让前端拿到权威值，而不是自己推测。
	status, err := training_capture.GetConsentStatus(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

// RevokeTrainingDataConsent 撤回同意（立即停止后续采集）。
//
// POST /api/user/training-data/revoke
//
// **语义边界**：撤回只停止**后续**采集，**不删除**已上传到云盘的数据。
// 这个边界必须对用户说清楚（前端小字），否则是误导性承诺。
// 真要删历史数据属于数据主体删除请求，需要人工流程（见 spec 的待办）。
func RevokeTrainingDataConsent(c *gin.Context) {
	userId := c.GetInt("id")
	if err := training_capture.RevokeConsent(userId); err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := training_capture.GetConsentStatus(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

// ---------- 管理端（T4.2） ----------

// TrainingDataOverview 管理端总览。
type TrainingDataOverview struct {
	// Gateway 总开关状态（含能否开启与原因）。
	Gateway training_capture.GatewayStatus `json:"gateway"`
	// Stats 今日采集统计（进程内计数，跨 UTC 日自动重置）。
	Stats training_capture.CaptureStatsSnapshot `json:"stats"`
	// Sink 云盘容量与规模。
	Sink SinkOverview `json:"sink"`
}

// SinkOverview 云盘用量（供前端卡片）。
type SinkOverview struct {
	Provider string `json:"provider"`
	// QuotaKnown 为 false 时下面的容量字段**不可信**（后端不支持或查询失败），
	// 前端应显示「未知」而不是把 0 当成「云盘满了」。
	QuotaKnown     bool  `json:"quota_known"`
	TotalBytes     int64 `json:"total_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	RemainingBytes int64 `json:"remaining_bytes"`
	// Folders/Files 是本程序根目录下的规模；Truncated 为 true 时是**下界**。
	Folders   int  `json:"folders"`
	Files     int  `json:"files"`
	Truncated bool `json:"truncated"`
	// LastError 非空说明本次查询失败（前端据此提示，而不是显示 0）。
	LastError string `json:"last_error,omitempty"`
}

// GetTrainingDataAdminOverview 管理端总览。
//
// GET /api/training-data/admin/overview
func GetTrainingDataAdminOverview(c *gin.Context) {
	overview := TrainingDataOverview{
		Gateway: training_capture.GetGatewayStatus(),
		Stats:   training_capture.GetCaptureStats(),
		Sink:    collectSinkOverview(c.Request.Context()),
	}
	common.ApiSuccess(c, overview)
}

// collectSinkOverview 查询云盘用量。
//
// **超时保护**：云盘 API 是外部依赖，慢或挂时不能让管理页面一直转圈。
// 给 10s 上限，超时就返回「未知」并附原因。
func collectSinkOverview(parent context.Context) SinkOverview {
	sink := training_capture.ActiveSink()
	out := SinkOverview{Provider: sink.Name()}

	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	stat, err := sink.Stat(ctx)
	if err != nil {
		// 不把错误当成功：前端显示「未知」并给出原因。
		out.LastError = err.Error()
		return out
	}
	out.QuotaKnown = stat.TotalBytes > 0
	out.TotalBytes = stat.TotalBytes
	out.UsedBytes = stat.UsedBytes
	out.RemainingBytes = stat.RemainingBytes
	out.Folders = stat.FolderCount
	out.Files = stat.FileCount

	// 目录/文件数在 OneDrive 后端上是可选能力（不是所有后端都能廉价统计），
	// 用**接口断言**探测而不是把它加进 TrainingDataSink 主接口 ——
	// 加进主接口会让每个后端（含未来的 S3/OSS/WebDAV）都必须实现它。
	if counter, ok := sink.(interface {
		CountItems(ctx context.Context, maxDepth int) (int, int, bool, error)
	}); ok {
		folders, files, truncated, err := counter.CountItems(ctx, 3)
		if err != nil {
			out.LastError = err.Error()
			return out
		}
		out.Folders, out.Files, out.Truncated = folders, files, truncated
	}
	return out
}

// GetTrainingDataByDate 按日期查询采集量。
//
// GET /api/training-data/admin/by-date?date=YYYY-MM-DD
//
// 注意：当前实现只返回**当日的进程内计数**（可精确到日），
// 历史日期无法回溯 —— 进程重启后进程内计数归零。
// 这一点在前端必须明示，否则管理员会以为「昨天没采到数据」。
func GetTrainingDataByDate(c *gin.Context) {
	date := strings.TrimSpace(c.Query("date"))
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			common.ApiErrorMsg(c, "invalid date, expected YYYY-MM-DD")
			return
		}
	}

	stats := training_capture.GetCaptureStats()
	today := stats.Day
	// 请求的日期不是今天 → 明确说明无法提供（而不是返回今天的数字冒充）。
	if date != "" && date != today {
		common.ApiSuccess(c, gin.H{
			"date":   date,
			"today":  today,
			"known":  false,
			"reason": "only the current day's counters are kept in memory; historical daily totals are not persisted",
			"stats":  nil,
		})
		return
	}
	common.ApiSuccess(c, gin.H{
		"date":  today,
		"today": today,
		"known": true,
		"stats": stats,
	})
}

// SetTrainingDataEnabled 管理端切换总开关。
//
// PUT /api/training-data/admin/enabled
//
// 请求体 `{"enabled":true}`。
func SetTrainingDataEnabled(c *gin.Context) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		common.ApiErrorMsg(c, "invalid request: expected {\"enabled\": true|false}")
		return
	}

	// **开启前必须校验云盘可用**：允许「打开」但实际无处可存，
	// 会让管理员以为在采集（数据全被丢弃）。
	if *body.Enabled && !training_capture.CanEnableGateway() {
		common.ApiErrorMsg(c, "cannot enable: training data sink is not configured")
		return
	}

	if err := training_capture.SetGatewayEnabledPersisted(*body.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	// 开关打开时确保 worker 在跑（幂等）；关闭时让它停消费。
	if *body.Enabled {
		training_capture.StartWorker()
	} else {
		training_capture.StopWorker()
	}

	common.ApiSuccess(c, training_capture.GetGatewayStatus())
}

// ---------- 小工具 ----------
