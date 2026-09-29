package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/service"
)

// 管理端数据库导出/导入（灾备）。
//
// 安全要点：
//   - 仅 RootAuth（导出含全站密钥/用户数据，权限必须最严）。
//   - 导出：流式写响应，不在内存里攒整份（生产 logs 50 万+ 行）。
//   - 导入：限制请求体大小、以「插入缺失行」语义（不删不覆盖），
//     导入后刷新内存缓存（option/渠道/定价），避免新数据不可见。

// maxBackupImportBytes 导入请求体上限（1 GiB）。防止畸形/恶意超大上传打爆内存与磁盘。
// 备份含日志库时可接近此值，故留足余量。
const maxBackupImportBytes = 1 << 30

// ExportDatabase GET /api/system/db/export
// query: include_logs=true 时一并导出日志库（体积大，默认不含）。
func ExportDatabase(c *gin.Context) {
	includeLogs := c.Query("include_logs") == "true"
	stamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("new-api-backup-%s.sqljson.gz", stamp)

	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// 备份是实时快照，不得被任何中间层缓存。
	c.Header("Cache-Control", "no-store")

	c.Status(http.StatusOK)
	summary, err := service.StreamDatabaseBackup(c.Writer, includeLogs)
	if err != nil {
		// 响应已开始写出，无法再改状态码；记录日志供排查，并尽力附加错误尾注。
		logger.LogError(c.Request.Context(), fmt.Sprintf("database export failed after streaming began: %v", err))
		return
	}
	recordManageAudit(c, "system.db_export", map[string]any{
		"include_logs": includeLogs,
		"tables":       len(summary.Tables),
		"total":        summary.Total,
	})
}

// ImportDatabase POST /api/system/db/import
// body: gzip 压缩的 JSON Lines 备份文件（multipart/form-data，字段名 file）。
// query: purge_logs=true 时导入前清空日志库（用于「日志已归档、只要账目」场景）。
//
// 语义：只插入不存在的行（冲突跳过），绝不删除或覆盖既有数据。
func ImportDatabase(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackupImportBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	src, err := fileHeader.Open()
	if err != nil {
		common.ApiErrorMsg(c, "failed to open uploaded backup")
		return
	}
	defer func() { _ = src.Close() }()

	result, err := service.ImportDatabaseBackup(src)
	if err != nil {
		// 区分「文件格式问题」（用户可自行修正）与「服务端问题」。
		common.ApiErrorMsg(c, "import failed: "+err.Error())
		return
	}

	// 导入后必须刷新内存态，否则新导入的 option/渠道/定价不会生效。
	service.RefreshRuntimeStateAfterImport()

	recordManageAudit(c, "system.db_import", map[string]any{
		"inserted": result.Total,
		"tables":   len(result.Inserted),
		"errors":   len(result.Errors),
	})

	common.ApiSuccess(c, gin.H{
		"inserted": result.Inserted,
		"skipped":  result.Skipped,
		"total":    result.Total,
		"errors":   result.Errors,
	})
}

// ExportDatabaseInfo GET /api/system/db/export/info
// 返回本次备份将包含的表清单与各表行数（不含数据），供管理端预览。
func ExportDatabaseInfo(c *gin.Context) {
	names := service.BackupTableNames()
	counts := make(map[string]int64, len(names))
	var total int64
	for _, name := range names {
		var n int64
		if err := modelCountRows(name, &n); err == nil {
			counts[name] = n
			total += n
		}
	}
	common.ApiSuccess(c, gin.H{
		"tables": names,
		"counts": counts,
		"total":  total,
		"hint":   "add ?include_logs=true to also export the log database",
	})
}

// modelCountRows 统计表行数（失败时返回错误，由调用方决定是否忽略）。
func modelCountRows(table string, out *int64) error {
	return service.CountTableRows(table, out)
}

// 兼容：保留 ctx 以便将来加超时；当前仅用于审计。
var _ = strconv.Itoa
