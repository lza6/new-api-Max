package controller

import (
	"context"
	"fmt"
	"strconv"
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
// 请求体可选 `{"policy_version":"<当前版本>"}`；**不传则用服务端当前版本**。
// 具体版本串以 `training_capture.CurrentPolicyVersion` 为准（会随声明改版递增），
// 这里不写死，避免注释与代码版本脱节。
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
		// 深度必须是「布局层数 + 1」：分片文件在**模型目录内部**，
		// 而 CountItems 只在「访问到的目录里」数文件。用布局层数（3）会让
		// 模型目录只被计入 folders、永不被展开 → files 恒为 0。
		// 见 `training_capture.CaptureLayoutDepth` 的说明。
		//
		// 走缓存：总览每 5s 轮询，而 CountItems 要遍历几十~几百个目录。
		// 不缓存的话「管理员开着页面」就会持续烧云盘配额，把采集 worker
		// 打到限流（两者共用配额）。缓存 TTL 见 trainingSinkCountsTTL。
		counts, cerr := trainingSinkCountsCache.get(func() (sinkCounts, error) {
			// 同样必须有界：不挂在请求 context 上，避免云盘挂起时
			// singleflight 把所有等待者永久阻塞。
			cctx, ccancel := context.WithTimeout(context.Background(), trainingCloudListTimeout)
			defer ccancel()
			folders, files, truncated, cErr := counter.CountItems(
				cctx, training_capture.CaptureLayoutDepth+1)
			if cErr != nil {
				return sinkCounts{}, cErr
			}
			return sinkCounts{Folders: folders, Files: files, Truncated: truncated}, nil
		})
		if cerr != nil {
			out.LastError = cerr.Error()
			return out
		}
		out.Folders, out.Files, out.Truncated = counts.Folders, counts.Files, counts.Truncated
	}
	return out
}

// GetTrainingDataByDate 按日期查询采集量。
//
// GET /api/admin/training-data/by-date?date=YYYY-MM-DD
//
// ## 口径（两条来源，必须让调用方能区分）
//
//  1. **当日进程内计数**（`stats`）：精确到「本次进程启动以来」的用户/对话/消息数，
//     但**重启即归零**——只有今天、且只覆盖本次启动之后。
//  2. **云盘目录树聚合**（`days`）：由一级目录（日期）聚合出的「每天多少用户/模型/分片/字节」，
//     是**持久且真实**的落盘记录，可回溯任意历史日期。
//
// 为什么不只留一个：两者回答的是不同问题 ——
// 「今天采了多少」（实时性，进程内更全）与「哪几天采了多少」（持久性，云盘才准）。
// 只给进程内计数会漏历史；只给云盘聚合会把「已上传」当成「已采集」
// （上传失败的增量不会出现在云盘里）。因此两个都给，并在字段名与文档里说清。
func GetTrainingDataByDate(c *gin.Context) {
	date := strings.TrimSpace(c.Query("date"))
	if date != "" {
		if !training_capture.IsCanonicalDate(date) {
			common.ApiErrorMsg(c, "invalid date, expected YYYY-MM-DD")
			return
		}
	}

	stats := training_capture.GetCaptureStats()
	today := stats.Day
	out := gin.H{
		"today": today,
		"stats": nil,
		"days":  []training_capture.DailyCaptureStat{},
	}

	// 云盘目录树聚合（管理员要的「几月几日采了多少」）。
	//
	// 走**进程内缓存 + 单飞**：这里是全量目录列举，而前端会轮询。
	// 直接每次列举会持续烧云盘速率配额，进而把**采集 worker** 打到 429
	// （两者共用同一份配额）—— 表现为「采集莫名开始失败」，根因却在管理页。
	if _, err := trainingDataReader(); err != nil {
		// 云盘后端不可用：不缓存错误，直接如实下发（恢复后立刻能看到数据）。
		out["daily_error"] = err.Error()
	} else {
		res, lerr := trainingDailyStatsCache.get(func() (trainingDailyStatsResult, error) {
			reader, rerr := trainingDataReader()
			if rerr != nil {
				return trainingDailyStatsResult{}, rerr
			}
			lister, ok := reader.(interface {
				ListTreeWithTruncation(ctx context.Context, root string, maxEntries int) ([]training_capture.TreeEntry, bool, error)
			})
			if !ok {
				return trainingDailyStatsResult{}, errSinkCannotReportTruncation
			}
			// **必须有界**：缓存加载不挂在任何请求的 context 上，若云盘挂起
			// 而这里用 Background，singleflight 会让**所有**等待者无限阻塞
			// （后续每个轮询请求都排在同一次永不返回的加载后面）。
			// 给一个总时限：超时即报错，错误不进缓存，下个请求会重新尝试。
			ctx, cancel := context.WithTimeout(context.Background(), trainingCloudListTimeout)
			defer cancel()
			entries, truncated, terr := lister.ListTreeWithTruncation(
				ctx, "", training_capture.ExportListTreeMaxEntries)
			if terr != nil {
				return trainingDailyStatsResult{}, terr
			}
			return trainingDailyStatsResult{
				Days:      training_capture.AggregateDailyStats(entries),
				Truncated: truncated,
			}, nil
		})
		if lerr != nil {
			// 云盘读不到不该让「今日计数」一起失败 —— 如实标注即可。
			out["daily_error"] = lerr.Error()
		} else {
			// 列举被截断 → 聚合出来的数字是**下界**，必须披露
			// （否则管理员会拿偏小的数当真实规模）。
			out["daily_truncated"] = res.Truncated
			if date != "" {
				// 指定日期：只返回那一天（没数据时返回空数组而非错误 ——
				// 「那天没采」是正常结果，不是故障）。
				filtered := make([]training_capture.DailyCaptureStat, 0, 1)
				for _, d := range res.Days {
					if d.Date == date {
						filtered = append(filtered, d)
					}
				}
				out["days"] = filtered
				out["date"] = date
			} else {
				out["days"] = res.Days
			}
		}
	}

	if date == "" || date == today {
		// 请求的日期是今天（或未指定）→ 附上进程内计数。
		out["stats"] = stats
		out["known"] = true
		// **必须在这里也写响应**：上面所有分支只是往 out 里填字段，
		// 只有函数末尾才真正下发。早期版本这里直接 `return`，
		// 导致「不传 date」（前端默认路径）与「查今天」两种最常见请求
		// 返回 HTTP 200 空 body —— 前端解析失败、每日历史卡 100% 报错，
		// 而历史日期分支却正常，所以很容易被误判成前端问题。
		common.ApiSuccess(c, out)
		return
	}
	// 历史日期：进程内计数不可得（重启归零），但云盘的 `days` 已给出真实数据。
	out["known"] = false
	out["reason"] = "in-memory counters only cover the current day; the `days` field is aggregated from the cloud drive and is authoritative for historical dates"
	common.ApiSuccess(c, out)
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

// ---------- 管理端：云盘文件浏览与操作（增量需求 B4 / B7） ----------

// trainingDataReader 取得当前后端的读取能力。
// 返回 nil 表示后端不支持读取（如 disabled）——调用方须给出明确错误。
func trainingDataReader() (training_capture.SinkReader, error) {
	sink := training_capture.ActiveSink()
	if reader, ok := sink.(training_capture.SinkReader); ok {
		return reader, nil
	}
	return nil, fmt.Errorf(
		"backend %q does not support reading (needed for browsing/analysis)", sink.Name())
}

// TrainingDataFileEntry 云盘文件项（对外展示）。
type TrainingDataFileEntry struct {
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir"`
	SizeBytes  int64  `json:"size_bytes"`
	ModifiedAt int64  `json:"modified_at"`
}

// ListTrainingDataFiles 列出云盘目录树。
//
// GET /api/training-data/admin/files?path=<可选子目录>&max=<可选条目上限>
//
// `path` 为空表示从配置的根目录开始。管理端据此渲染可下钻的文件树。
func ListTrainingDataFiles(c *gin.Context) {
	reader, err := trainingDataReader()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	root := strings.Trim(strings.TrimSpace(c.Query("path")), "/")
	// 纵深防御：与预览/删除/分析三个端点**同一判据**。
	// 虽然本接口仅管理员可达，但路径会经 `itemPath()` 拼到 RootPath 之后
	// 交给 Graph —— `..` 可让列举逃出 RootPath 指向云盘其它目录，
	// 与本功能「只读训练数据目录」的边界不符。四个入口用同一条规则，
	// 避免将来新增端点时漏掉一个。
	if strings.Contains(root, "..") {
		common.ApiErrorMsg(c, "invalid path")
		return
	}
	maxEntries := 0
	if raw := c.Query("max"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			maxEntries = v
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	entries, err := reader.ListTree(ctx, root, maxEntries)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// [修复防御] 管理员**浏览用户对话数据**必须留痕。
	//
	// 采集的是用户与模型的对话内容（可能含密钥、源码、个人信息），
	// 而管理员能看任意用户的数据。没有审计就没法回答
	// 「谁在什么时候看过谁的数据」—— 这是内部人滥用无法追查的缺口
	// （盲点扫描发现）。与 OAuth 迁移用同一套审计设施。
	recordUserSecurityAudit(c, c.GetInt("id"), "admin.training_data.list_files", map[string]any{
		"success": true,
		"root":    root,
		"count":   len(entries),
	})

	out := make([]TrainingDataFileEntry, 0, len(entries))
	var totalBytes int64
	for _, e := range entries {
		out = append(out, TrainingDataFileEntry{
			Path: e.Path, IsDir: e.IsDir,
			SizeBytes: e.SizeBytes, ModifiedAt: e.ModifiedAt,
		})
		if !e.IsDir {
			totalBytes += e.SizeBytes
		}
	}

	common.ApiSuccess(c, gin.H{
		"root":        root,
		"entries":     out,
		"count":       len(out),
		"total_bytes": totalBytes,
		// 达到上限说明列表**不完整**，前端须提示（不能当成全部）。
		"truncated": maxEntries > 0 && len(out) >= maxEntries,
	})
}

// PreviewTrainingDataFile 读取并预览一个分片（解压后的 JSONL 文本）。
//
// GET /api/training-data/admin/file?path=...&max_bytes=...
//
// 返回**解压后**的行数组（云盘上存的是 gzip）。同时如实披露坏行数与
// 截断事实 —— 管理端不能把残缺数据当完整展示。
func PreviewTrainingDataFile(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		common.ApiErrorMsg(c, "missing required query parameter: path")
		return
	}
	if strings.Contains(path, "..") {
		// 纵深防御：即使 sink 侧也做了净化，这里再拒一次上跳路径。
		common.ApiErrorMsg(c, "invalid path")
		return
	}

	reader, err := trainingDataReader()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	maxBytes := int64(0)
	if raw := c.Query("max_bytes"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			maxBytes = v
		}
	}
	// **夹取上限**：不夹取的话管理员传 max_bytes=50000000000 就能绕过
	// reader 的默认上限，把巨型文件整个读进内存（与全仓「无界 io.ReadAll」
	// 治理相悖）。夹到 64MB —— 压缩分片解压后远小于此。
	const previewMaxBytesLimit = int64(64) << 20
	if maxBytes <= 0 || maxBytes > previewMaxBytesLimit {
		maxBytes = previewMaxBytesLimit
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	raw, err := reader.ReadFile(ctx, path, maxBytes)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 同上：**预览即读取用户对话正文**，是比列目录更敏感的访问，必须留痕。
	recordUserSecurityAudit(c, c.GetInt("id"), "admin.training_data.preview_file", map[string]any{
		"success": true,
		"path":    path,
		"bytes":   len(raw),
	})

	// 分片是 gzip（见 compression.go）。兼容历史明文分片：
	// 解压失败就按明文处理 —— 不因为格式演进就让旧数据读不了。
	plain := raw
	compressed := false
	if len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b { // gzip magic
		decompressed, derr := training_capture.DecompressPart(raw)
		if derr != nil {
			common.ApiError(c, fmt.Errorf("decompress %s: %w", path, derr))
			return
		}
		plain = decompressed
		compressed = true
	}

	lines := make([]string, 0, 64)
	for _, ln := range strings.Split(string(plain), "\n") {
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, ln)
		}
	}

	common.ApiSuccess(c, gin.H{
		"path":             path,
		"compressed":       compressed,
		"compressed_bytes": len(raw),
		"plain_bytes":      len(plain),
		"lines":            lines,
		"line_count":       len(lines),
	})
}

// DeleteTrainingDataFile 删除一个分片。
//
// DELETE /api/training-data/admin/file?path=...
//
// 危险操作：HTTP 层要求前端二次确认，这里额外要求显式 `confirm=1`，
// 避免误调用（例如把查询串拼错）造成静默删数据。
func DeleteTrainingDataFile(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		common.ApiErrorMsg(c, "missing required query parameter: path")
		return
	}
	if strings.Contains(path, "..") {
		common.ApiErrorMsg(c, "invalid path")
		return
	}
	if c.Query("confirm") != "1" {
		common.ApiErrorMsg(c, "destructive operation requires confirm=1")
		return
	}
	// 只允许删文件（以分片/压缩后缀结尾），不允许删目录字符串。
	if !strings.HasSuffix(path, ".jsonl.gz") && !strings.HasSuffix(path, ".jsonl") {
		common.ApiErrorMsg(c, "only part files (.jsonl.gz / .jsonl) can be deleted via this endpoint")
		return
	}

	reader, err := trainingDataReader()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := reader.DeleteFile(ctx, path); err != nil {
		common.ApiError(c, err)
		return
	}
	common.SysLog("training data capture: admin deleted remote part " + path)
	common.ApiSuccess(c, gin.H{"deleted": path})
}
