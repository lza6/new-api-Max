package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service/training_capture"
	"github.com/lza6/new-api-Max/setting/system_setting"
)

// 训练数据「一键导出 + 分片下载 + 断点续传 + 下载记录」的 HTTP 接口。
//
// ## 安全契约（每条对应一个真实攻击面）
//
//  1. **管理端全部走 `AdminAuth()`**（路由处声明）。导出是**用户数据**，
//     只有管理员可达；路由注册在 `/api/admin/training-data/export*`。
//  2. **导出 ID 用数据库主键**，但对外的**下载直链**必须带限时签名 token
//     （`training_capture.IssueExportAccess`）——导出 ID 本身不是凭据。
//  3. **下载记录的下载人只从会话取**，绝不从请求体/查询串取。
//  4. **产物路径不由用户输入拼接**（`ExportStore.PathFor` 只吃数字 ID）。
//
// ## 断点续传的实现方式
//
// 产物是**本地上盘的普通文件**（可随机寻址），下载走 `http.ServeContent` ——
// 它原生处理 Range / If-Range / If-Modified-Since / 206 / 416 / Accept-Ranges。
// 但在调用它之前，本层自己解析 Range 做**下载记录**（记录本次下发区间与字节数），
// 并把「不支持的多区间」等非法请求**显式**处理（见 `ParseSingleRange`）。
// 不用 std 库的 range 解析是因为要拿到「本次区间」用于审计。

// ---------- 创建导出 ----------

// CreateTrainingDataExportRequest 创建导出请求体。
type CreateTrainingDataExportRequest struct {
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	UserId   int    `json:"user_id"`
	Model    string `json:"model"`
}

// maxExportRangeDays 单次导出的最大天数跨度（防误填整个历史）。
const maxExportRangeDays = 366

// CreateTrainingDataExport 创建一个导出任务并同步打包（目前为同步；产物就绪即返回）。
//
// POST /api/admin/training-data/export
//
// 请求体 `{"date_from":"2026-10-01","date_to":"2026-10-14","user_id":0,"model":""}`。
//
// **同步语义的诚实说明**：当前实现在请求内完成打包（小规模导出够用），
// 大范围导出会让请求长时间占用连接。真正的大规模场景应改为后台任务 +
// 轮询状态（本表已有 pending/building 状态位，便于将来演进）。
func CreateTrainingDataExport(c *gin.Context) {
	var body CreateTrainingDataExportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiErrorMsg(c, "invalid request body: expected {date_from,date_to,user_id?,model?}")
		return
	}

	from, to, err := training_capture.ValidateExportRange(body.DateFrom, body.DateTo, maxExportRangeDays)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	reader, err := trainingDataReader()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !training_capture.ExportStoreInstance().Enabled() {
		common.ApiErrorMsg(c, "training export store is not available on this deployment")
		return
	}

	adminId := c.GetInt("id")

	// 先落一条 pending 记录（拿到自增 ID 用于产物命名与直链绑定）。
	rec := &model.TrainingDataExport{
		Status:        model.TrainingExportStatusPending,
		DateFrom:      from,
		DateTo:        to,
		UserId:        body.UserId,
		Model:         strings.TrimSpace(body.Model),
		ArchiveFormat: "tar",
		CreatedBy:     adminId,
	}
	if err := model.CreateTrainingDataExport(rec); err != nil {
		common.ApiError(c, err)
		return
	}

	// 标记 building（若后续崩溃，重启时可按此状态清理/重试）。
	if err := model.UpdateTrainingDataExport(rec.ID, map[string]any{
		"status": model.TrainingExportStatusBuilding,
	}); err != nil {
		common.ApiError(c, err)
		return
	}

	// 打包。给一个宽松超时：几十 GB 从云盘拉取可能很久。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Hour)
	defer cancel()

	manifest, buildErr := buildExport(ctx, reader, rec)
	if buildErr != nil {
		_ = model.UpdateTrainingDataExport(rec.ID, map[string]any{
			"status":    model.TrainingExportStatusFailed,
			"error_msg": buildErr.Error(),
		})
		common.ApiError(c, fmt.Errorf("export failed: %w", buildErr))
		return
	}

	// 产物统计。
	store := training_capture.ExportStoreInstance()
	var size int64
	if f, info, openErr := store.Open(rec.ID); openErr == nil {
		size = info.Size()
		_ = f.Close()
	}

	_ = model.UpdateTrainingDataExport(rec.ID, map[string]any{
		"status":       model.TrainingExportStatusReady,
		"object_path":  fmt.Sprintf("export-%d.tar", rec.ID),
		"size_bytes":   size,
		"part_count":   manifest.PartCount,
		"record_count": manifest.RecordCount,
		"error_msg":    "",
	})

	out, err := model.GetTrainingDataExport(rec.ID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, out)
}

// buildExport 枚举云盘 → 筛选分片 → 打包。返回 manifest 供回填统计。
func buildExport(ctx context.Context, reader training_capture.SinkReader, rec *model.TrainingDataExport) (training_capture.ExportManifest, error) {
	var manifest training_capture.ExportManifest

	// [修复防御] 枚举必须用**带截断报告**的版本，并在截断时**明确报错**。
	//
	// 盲点 B13：`ListTree` 内部会把条目夹到全局硬顶（5000），
	// 而导出请求的是 100000。旧写法只拿到列表、无从知道被截断，
	// 于是会产出一个**看似完整、实为前 5000 项**的归档，
	// manifest 里的 PartCount 还是那个偏小的数 ——
	// 管理员拿它去训练，以为拿到了全量数据。
	//
	// 明确失败比产出半包好：半包不会报错，只会静默少数据。
	truncator, ok := reader.(interface {
		ListTreeWithTruncation(ctx context.Context, root string, maxEntries int) ([]training_capture.TreeEntry, bool, error)
	})
	var (
		entries   []training_capture.TreeEntry
		truncated bool
		err       error
	)
	if ok {
		entries, truncated, err = truncator.ListTreeWithTruncation(ctx, "", training_capture.ExportListTreeMaxEntries)
	} else {
		// 后端不支持截断报告：**保守处理** —— 无法证明完整性就不导出，
		// 而不是赌它没截断。
		return manifest, fmt.Errorf(
			"the configured sink cannot report listing truncation; refusing to build an export that may be silently incomplete")
	}
	if err != nil {
		return manifest, fmt.Errorf("list cloud drive: %w", err)
	}
	if truncated {
		// [修复防御] **不报具体数字**。
		//
		// 原错误串写「limit 100000 entries reached」，但 sink 内部会把
		// 请求值夹到自己的硬顶（OneDrive 当前是 5000）——
		// 于是那个 100000 永远不成立，运维照它调参只会更困惑。
		// 报一个自己无法核实的数字，比不给数字更糟。
		return manifest, fmt.Errorf(
			"cloud drive listing was truncated: the storage backend's own listing limit was reached, " +
				"or deep directories were not expanded, so the export would be silently incomplete; " +
				"narrow the date range or add a user/model filter")
	}

	sel := training_capture.ExportSelection{
		DateFrom: rec.DateFrom,
		DateTo:   rec.DateTo,
		UserId:   rec.UserId,
		Model:    rec.Model,
	}
	parts := training_capture.SelectExportParts(entries, sel)
	if len(parts) == 0 {
		// 空导出不是错误，但要明确告知（生成一个只含 manifest 的归档，
		// 让管理员能确认「范围里确实没数据」，而不是一个 500）。
		common.SysLog(fmt.Sprintf("training export %d: no parts matched %s..%s", rec.ID, rec.DateFrom, rec.DateTo))
	}

	store := training_capture.ExportStoreInstance()
	builder := &training_capture.ExportBuilder{
		Reader:   reader,
		Provider: training_capture.ActiveSink().Name(),
	}
	_, manifest, err = builder.Build(ctx, sel, parts, store.PathFor(rec.ID))
	if err != nil {
		return manifest, err
	}
	return manifest, nil
}

// ---------- 列出导出 / 详情 ----------

// ListTrainingDataExports 分页列出导出任务。
//
// GET /api/admin/training-data/export?offset=0&limit=50
func ListTrainingDataExports(c *gin.Context) {
	offset := parseNonNegInt(c.Query("offset"), 0)
	limit := parseNonNegInt(c.Query("limit"), 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, total, err := model.ListTrainingDataExports(offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"exports": rows,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

// GetTrainingDataExportDetail 读取单个导出 + 其下载记录。
//
// GET /api/admin/training-data/export/detail?id=<exportID>
func GetTrainingDataExportDetail(c *gin.Context) {
	id, ok := parseExportID(c)
	if !ok {
		return
	}
	rec, err := model.GetTrainingDataExport(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if rec == nil {
		common.ApiError(c, training_capture.ErrExportNotFound)
		return
	}
	downloads, err := model.ListTrainingDataExportDownloads(id, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"export":    rec,
		"downloads": downloads,
	})
}

// ---------- 直链 ----------

// IssueTrainingDataExportLink 为导出签发一个限时直链。
//
// POST /api/admin/training-data/export/link
// 请求体 `{"id":123,"ttl_seconds":3600}`。
//
// 返回带签名 token 的绝对 URL。**token 绑定导出 ID 且限时**（见 service 层说明）。
func IssueTrainingDataExportLink(c *gin.Context) {
	var body struct {
		ID         uint `json:"id"`
		TTLSeconds int  `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ID == 0 {
		common.ApiErrorMsg(c, "invalid request: expected {\"id\": <exportID>}")
		return
	}
	rec, err := model.GetTrainingDataExport(body.ID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if rec == nil {
		common.ApiError(c, training_capture.ErrExportNotFound)
		return
	}
	if rec.Status != model.TrainingExportStatusReady {
		common.ApiErrorMsg(c, "export is not ready for download (status: "+rec.Status+")")
		return
	}

	// 签发者 = 当前管理员（从会话取）。纳入签名并随 URL 带出，
	// 使无会话的直链下载也能归属到人（见 export_sign.go 的归属说明）。
	issuerID := c.GetInt("id")
	token, expiresAt, err := training_capture.IssueExportAccess(
		rec.ID, issuerID, training_capture.ClampExportTTL(body.TTLSeconds), time.Now().Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	base := exportDownloadBaseURL(c)
	url := fmt.Sprintf("%s/api/training-data/export/download?id=%d&iss=%d&expires=%d&token=%s",
		base, rec.ID, issuerID, expiresAt, token)

	common.ApiSuccess(c, gin.H{
		"url":        url,
		"expires_at": expiresAt,
		"export_id":  rec.ID,
		"issuer_id":  issuerID,
		"size_bytes": rec.SizeBytes,
		"filename":   fmt.Sprintf("training-export-%d.tar", rec.ID),
		// 明确告知客户端是否可续传（用户名字面要求「把是否支持断点续传告诉用户」）。
		"resumable":     true,
		"accept_ranges": "bytes",
	})
}

// exportDownloadBaseURL 计算下发 URL 的基地址。
//
// 优先部署配置的 `ServerAddress`（与站点公网地址一致），否则用请求自身的
// scheme+host。只取 scheme/host 拼本网关自指 URL，不把 Host 用于跳转。
func exportDownloadBaseURL(c *gin.Context) string {
	if base := strings.TrimSpace(system_setting.ServerAddress); base != "" {
		return strings.TrimRight(base, "/")
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// ---------- 下载（Range / 断点续传） ----------

// DownloadTrainingDataExport 下发产物，支持 Range 断点续传。
//
// GET /api/training-data/export/download?id=<exportID>&expires=<unix>&token=<sig>
//
// **注意**：本路由**不在 AdminAuth 之下**（否则直链给 curl/aria2 用时
// 会因无会话 Cookie 而 401）。鉴权完全依赖 token（限时 + 绑定导出 ID）。
// 路由与 AdminAuth 的关系见报告中的路由行。
//
// 行为：
//   - 无 Range → 200 全量；
//   - 合法单区间 Range → 206 + `Content-Range: bytes s-e/size`；
//   - 多区间 / 非法 Range → 忽略 Range，返回 200 全量（RFC 7233）；
//   - 越界 Range → 416 + `Content-Range: bytes */size`。
func DownloadTrainingDataExport(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Query("id")), 10, 64)
	if err != nil || id == 0 {
		common.ApiErrorMsg(c, "invalid export id")
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	// iss = 签发链接的管理员 ID（明文随行但参与签名，不可伪造）。
	issuerID, issErr := strconv.Atoi(strings.TrimSpace(c.Query("iss")))
	if issErr != nil {
		issuerID = 0
	}
	if !training_capture.VerifyExportAccess(token, uint(id), issuerID, time.Now().Unix()) {
		// 401：直链无效/过期。不区分「过期」与「伪造」。
		c.Header("WWW-Authenticate", `Bearer realm="training-export"`)
		c.AbortWithStatusJSON(401, gin.H{"success": false, "message": "invalid or expired download token"})
		return
	}

	rec, err := model.GetTrainingDataExport(uint(id))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if rec == nil || rec.Status != model.TrainingExportStatusReady {
		common.ApiError(c, training_capture.ErrExportNotFound)
		return
	}

	store := training_capture.ExportStoreInstance()
	f, info, err := store.Open(uint(id))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer f.Close()

	size := info.Size()
	filename := fmt.Sprintf("training-export-%d.tar", id)

	// ETag：用大小+修改时间构造（产物是不可变文件，够用且无需读内容）。
	etag := fmt.Sprintf("\"%x-%x\"", size, info.ModTime().UnixNano())

	// 记录下载（开始）。有会话时以会话为准；无会话（curl/aria2 直链）
	// 时归到签发者（见 export_sign.go 的归属说明与边界）。
	downloaderID := c.GetInt("id")
	if downloaderID == 0 {
		downloaderID = issuerID
	}
	dl := &model.TrainingDataExportDownload{
		ExportId:     uint(id),
		DownloaderId: downloaderID,
		Source:       downloadSource(c),
		RangeHeader:  truncateHeader(c.GetHeader("Range")),
		StartedAt:    time.Now().Unix(),
	}
	if err := model.CreateTrainingDataExportDownload(dl); err != nil {
		// 记录失败不阻断下载，但如实记日志（不能静默吞）。
		common.SysError("training export: failed to record download start: " + err.Error())
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", "application/x-tar")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("ETag", etag)
	// 产物不可变，可长缓存但要求重新验证（配合 Range 续传友好）。
	c.Header("Cache-Control", "private, max-age=0, must-revalidate")

	// If-Range 不匹配 → 丢弃 Range（返回全量），避免续传出损坏文件。
	rangeHeader := c.GetHeader("Range")
	if rangeHeader != "" && !training_capture.IfRangeMatches(c.GetHeader("If-Range"), info.ModTime(), etag) {
		rangeHeader = ""
	}

	// 自己解析本次区间（用于下载记录与非法/越界请求处理），再交给 ServeContent 下发。
	// ServeContent 会再次解析同一个 Range —— 两次解析同一头是幂等的，
	// 且我们借此拿到审计所需的区间信息。
	if rangeHeader != "" {
		_, perr := training_capture.ParseSingleRange(rangeHeader, size)
		switch {
		case perr == nil:
			// 合法单区间：交给 ServeContent 下发 206。
		case errors.Is(perr, training_capture.ErrRangeNotSatisfiable):
			// 越界：416 + 明确 Content-Range（RFC 7233 §4.4）。
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", size))
			c.Status(http.StatusRequestedRangeNotSatisfiable)
			finalizeDownload(dl.ID, 0, false)
			return
		default:
			// 非法 / 多区间：**忽略 Range**（RFC 7233 §3.1：必须忽略非法 Range），
			// 清掉请求头让 ServeContent 走全量 200。
			c.Request.Header.Del("Range")
			rangeHeader = ""
		}
	}

	// 用计数 writer 记录真实下发字节数（含 206 的分段）。
	// 复用 controller 既有的 countingResponseWriter（它把字节数累加到指针）。
	var sent int64
	counter := &countingResponseWriter{ResponseWriter: c.Writer, bytes: &sent}
	http.ServeContent(counter, c.Request, filename, info.ModTime(), f)

	// 完成判定：写成功（有字节）且请求上下文未被中断。
	completed := sent > 0 && c.Request.Context().Err() == nil
	finalizeDownload(dl.ID, sent, completed)

	// 自增导出下载计数（只在成功下发后）。
	if err := model.IncrementTrainingDataExportDownloadCount(uint(id)); err != nil {
		common.SysError("training export: failed to bump download count: " + err.Error())
	}
}

// finalizeDownload 回填下载记录（尽力而为，失败只记日志）。
func finalizeDownload(dlID uint, bytes int64, completed bool) {
	if dlID == 0 {
		return
	}
	if err := model.CompleteTrainingDataExportDownload(dlID, bytes, completed, time.Now().Unix()); err != nil {
		common.SysError("training export: failed to record download result: " + err.Error())
	}
}

// downloadSource 判定下载方式：带 Range 头视为断点续传/工具下载，否则浏览器直连。
func downloadSource(c *gin.Context) string {
	if c.GetHeader("Range") != "" {
		return "direct"
	}
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	if strings.Contains(ua, "curl") || strings.Contains(ua, "aria2") || strings.Contains(ua, "wget") {
		return "direct"
	}
	return "browser"
}

// truncateHeader 截断过长的头值（Range 头有界，但防御性截断防超长入参）。
func truncateHeader(v string) string {
	const maxLen = 120
	if len(v) > maxLen {
		return v[:maxLen]
	}
	return v
}

// ---------- 删除导出 ----------

// DeleteTrainingDataExport 删除导出任务及其产物。
//
// DELETE /api/admin/training-data/export?id=<id>&confirm=1
func DeleteTrainingDataExport(c *gin.Context) {
	id, ok := parseExportID(c)
	if !ok {
		return
	}
	if c.Query("confirm") != "1" {
		common.ApiErrorMsg(c, "destructive operation requires confirm=1")
		return
	}
	// 删文件（尽力）再删记录。
	training_capture.ExportStoreInstance().Remove(id)
	if err := model.DB.Where("id = ?", id).Delete(&model.TrainingDataExport{}).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	// 连带删除其下载记录（否则会留下悬空 export_id）。
	if err := model.DB.Where("export_id = ?", id).Delete(&model.TrainingDataExportDownload{}).Error; err != nil {
		common.SysError("training export: failed to delete download records: " + err.Error())
	}
	common.ApiSuccess(c, gin.H{"deleted": id})
}

// ---------- 小工具 ----------

// parseExportID 解析查询串里的导出 id。
func parseExportID(c *gin.Context) (uint, bool) {
	raw := strings.TrimSpace(c.Query("id"))
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		common.ApiErrorMsg(c, "invalid or missing export id")
		return 0, false
	}
	return uint(v), true
}

// parseNonNegInt 解析非负整数，非法或空返回默认值。
func parseNonNegInt(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return def
	}
	return v
}
