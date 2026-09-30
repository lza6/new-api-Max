package service

import (
	"bufio"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 数据库导出/导入（管理员灾备）。
//
// 设计取舍：
//   - **纯 Go 实现**，不依赖容器内的 pg_dump/mysqldump/sqlite3（生产镜像是
//     debian-slim，未装这些工具；装了也会带来版本漂移与跨库差异）。
//   - **JSON Lines + gzip 流式**：每行一条记录、逐表逐行写入，内存占用与表大小
//     无关（生产 logs 表 50 万+ 行也能安全导出）。三库（SQLite/MySQL/PostgreSQL）
//     走同一份 GORM 代码，无方言分支。
//   - **导入用「插入缺失行」语义**（存在即跳过），不做覆盖删除——避免误导入
//     清空生产数据。这是一份「灾备/迁移」工具，不是「同步」工具。

const (
	// backupFormatVersion 导出格式版本，便于将来演进时识别旧文件。
	backupFormatVersion = 1
	// backupKind 文件内的类型标识。
	backupKind = "new-api-db-backup"
	// maxImportLineBytes 单行上限（防畸形文件把内存吃爆）。16MiB 足以容纳
	// 单条超长 content/other JSON，同时挡住恶意的无换行大文件。
	maxImportLineBytes = 16 << 20
)

// BackupMeta 导出文件头部元信息（第一行）。
type BackupMeta struct {
	Kind        string `json:"kind"`
	Version     int    `json:"version"`
	CreatedAt   string `json:"created_at"`
	AppVersion  string `json:"app_version"`
	DBType      string `json:"db_type"`
	IncludeLogs bool   `json:"include_logs"`
}

// BackupTableHeader 每个表段开始前的一行（表名 + 预期行数，仅参考）。
type BackupTableHeader struct {
	Table string `json:"__table__"`
	Count int64  `json:"count"`
}

// BackupSummary 导出结果统计。
type BackupSummary struct {
	Tables map[string]int64 `json:"tables"`
	Total  int64            `json:"total"`
}

// BackupImportResult 导入结果统计。
type BackupImportResult struct {
	Inserted map[string]int64 `json:"inserted"`
	Skipped  map[string]int64 `json:"skipped"`
	Errors   []string         `json:"errors,omitempty"`
	Total    int64            `json:"total"`
}

// StreamDatabaseBackup 把主库（可选日志库）导出为 gzip 压缩的 JSON Lines 流。
// 逐表逐行写出，适合直接管道到 HTTP 响应体。
func StreamDatabaseBackup(w io.Writer, includeLogs bool) (*BackupSummary, error) {
	gz := gzip.NewWriter(w)
	defer func() { _ = gz.Close() }()
	bw := bufio.NewWriterSize(gz, 64*1024)

	meta := BackupMeta{
		Kind:        backupKind,
		Version:     backupFormatVersion,
		CreatedAt:   time.Now().Format(time.RFC3339),
		AppVersion:  common.Version,
		DBType:      string(common.MainDatabaseType()),
		IncludeLogs: includeLogs,
	}
	if err := writeJSONLine(bw, meta); err != nil {
		return nil, fmt.Errorf("write backup meta: %w", err)
	}

	summary := &BackupSummary{Tables: map[string]int64{}}
	if err := streamTables(bw, model.DB, model.BackupTables(), summary); err != nil {
		return nil, err
	}
	if includeLogs {
		if err := streamTables(bw, model.LOG_DB, model.LogTables(), summary); err != nil {
			return nil, err
		}
	}

	if err := bw.Flush(); err != nil {
		return nil, fmt.Errorf("flush backup: %w", err)
	}
	// 显式关闭 gzip 以写入尾部校验和；defer 里的 Close 会因重复调用而无害。
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return summary, nil
}

func streamTables(w *bufio.Writer, db *gorm.DB, tables []any, summary *BackupSummary) error {
	for _, proto := range tables {
		if db == nil {
			continue
		}
		table := gormTableName(db, proto)
		var count int64
		if err := db.Model(proto).Count(&count).Error; err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if err := writeJSONLine(w, BackupTableHeader{Table: table, Count: count}); err != nil {
			return fmt.Errorf("write header for %s: %w", table, err)
		}
		// 逐行扫描：Find 到 map 切片会一次性载入内存，故用 Rows 流式读取。
		rows, err := db.Model(proto).Rows()
		if err != nil {
			return fmt.Errorf("scan %s: %w", table, err)
		}
		written, scanErr := streamRows(w, rows)
		_ = rows.Close()
		if scanErr != nil {
			return fmt.Errorf("stream %s: %w", table, scanErr)
		}
		summary.Tables[table] = written
		summary.Total += written
	}
	return nil
}

// streamRows 把 sql.Rows 逐行转成 JSON 行写出，返回写入行数。
func streamRows(w *bufio.Writer, rows *sql.Rows) (int64, error) {
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	var written int64
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return written, err
		}
		record := make(map[string]any, len(cols))
		for i, col := range cols {
			record[col] = normalizeDBValue(values[i])
		}
		if err := writeJSONLine(w, record); err != nil {
			return written, err
		}
		written++
	}
	return written, rows.Err()
}

// normalizeDBValue 把驱动返回的 []byte 转成 string，保证 JSON 可读且可回写。
// 同时把 time.Time 归一为 RFC3339 字符串，避免三库时间格式差异。
func normalizeDBValue(v any) any {
	switch typed := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return typed
	}
}

func writeJSONLine(w *bufio.Writer, v any) error {
	raw, err := common.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

// gormTableName 解析模型对应的表名。
func gormTableName(db *gorm.DB, proto any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(proto); err != nil {
		return fmt.Sprintf("%T", proto)
	}
	return stmt.Schema.Table
}

// ImportDatabaseBackup 从 gzip 压缩的 JSON Lines 流导入数据。
//
// 语义：**只插入不存在的行**（冲突跳过），不删除、不覆盖已有数据。
// 因此可以安全地对一份「可能已部分导入」的备份重复执行。
func ImportDatabaseBackup(r io.Reader) (*BackupImportResult, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, errors.New("文件不是有效的 gzip 备份（或已损坏）")
	}
	defer func() { _ = gz.Close() }()

	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 0, 64*1024), maxImportLineBytes)

	result := &BackupImportResult{Inserted: map[string]int64{}, Skipped: map[string]int64{}}

	// 第一行必须是 meta
	if !scanner.Scan() {
		return nil, errors.New("备份文件为空")
	}
	var meta BackupMeta
	if err := common.Unmarshal(scanner.Bytes(), &meta); err != nil || meta.Kind != backupKind {
		return nil, errors.New("备份文件格式不正确（缺少有效头部）")
	}
	if meta.Version > backupFormatVersion {
		return nil, fmt.Errorf("备份格式版本 %d 高于当前支持的 %d，请升级后再导入", meta.Version, backupFormatVersion)
	}

	tableTables := backupTableIndex()
	currentTable := ""
	var pending []map[string]any

	flush := func() error {
		if currentTable == "" || len(pending) == 0 {
			pending = pending[:0]
			return nil
		}
		inserted, skipped, err := insertRows(currentTable, pending)
		result.Inserted[currentTable] += inserted
		result.Skipped[currentTable] += skipped
		result.Total += inserted
		if err != nil {
			// 单表失败不中断整体导入：记录并继续，让调用方看到全貌。
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", currentTable, err))
		}
		pending = pending[:0]
		return nil
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// 表段头：{"__table__":"...","count":N}
		var header BackupTableHeader
		if common.Unmarshal(line, &header) == nil && header.Table != "" {
			if err := flush(); err != nil {
				return nil, err
			}
			if _, ok := tableTables[header.Table]; !ok {
				// 未知表（可能是更新版本导出的）：跳过其数据行，避免误写。
				currentTable = ""
				continue
			}
			currentTable = header.Table
			continue
		}
		if currentTable == "" {
			continue // 未知表段的数据行，忽略
		}
		var record map[string]any
		if err := common.Unmarshal(line, &record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: 跳过无法解析的行", currentTable))
			continue
		}
		pending = append(pending, record)
		// 分批落库，避免超长表段把内存吃爆。
		if len(pending) >= 500 {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取备份流失败: %w", err)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return result, nil
}

// insertRows 把一批记录写入指定表，冲突（主键已存在）跳过。
func insertRows(table string, records []map[string]any) (inserted, skipped int64, err error) {
	if len(records) == 0 {
		return 0, 0, nil
	}
	// [fix-mysql] 三库冲突跳过必须分支处理，不能只靠 clause.OnConflict：
	// GORM 在 **MySQL** 上把 `clause.OnConflict{DoNothing:true}` + map 数据渲染成
	// `ON DUPLICATE KEY UPDATE `（尾随空格 + 空更新子句）→ **MySQL 语法错误**
	// （Error 1064），导入在 MySQL 上完全不可用。三库矩阵测试实测证实。
	// 正确做法：MySQL 用 INSERT IGNORE 表修饰符，PostgreSQL/SQLite 用
	// ON CONFLICT DO NOTHING（由 clause.OnConflict 正确渲染）。
	var tx *gorm.DB
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		tx = model.DB.Table(table).Clauses(clause.Insert{Modifier: "IGNORE"}).CreateInBatches(records, 200)
	} else {
		tx = model.DB.Table(table).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(records, 200)
	}
	if tx.Error != nil {
		return 0, 0, tx.Error
	}
	// CreateInBatches 的 RowsAffected 即真正插入的行数，其余为冲突跳过。
	inserted = tx.RowsAffected
	skipped = int64(len(records)) - inserted
	if skipped < 0 {
		skipped = 0
	}
	return inserted, skipped, nil
}

// backupTableIndex 返回「表名 → 模型」的索引，用于导入时的表名校验。
func backupTableIndex() map[string]any {
	index := map[string]any{}
	for _, proto := range append(model.BackupTables(), model.LogTables()...) {
		name := gormTableName(model.DB, proto)
		index[name] = proto
	}
	return index
}

// BackupTableNames 供管理端 UI 展示本次备份将包含的表。
func BackupTableNames() []string {
	names := make([]string, 0)
	for _, proto := range model.BackupTables() {
		names = append(names, gormTableName(model.DB, proto))
	}
	return names
}

func init() {
	// 导入时的冲突策略在 model 包里定义（依赖 clause），这里只做启动期自检：
	// 确保表清单非空，避免将来误改导致空备份。
	if len(model.BackupTables()) == 0 {
		logger.LogError(nil, "backup: model.BackupTables() is empty; database backup would produce an empty file")
	}
	if strings.TrimSpace(common.Version) == "" {
		logger.LogWarn(nil, "backup: common.Version is empty; backup meta will lack an app version")
	}
}

// CountTableRows 统计指定表行数（供管理端备份预览）。
func CountTableRows(table string, out *int64) error {
	return model.DB.Table(table).Count(out).Error
}

// RefreshRuntimeStateAfterImport 导入数据后刷新进程内缓存，让新写入的
// option / 渠道 / 定价 / 用户等立即生效，避免「导入了却看不到」。
// 复用既有的缓存失效入口，不新增缓存层。
func RefreshRuntimeStateAfterImport() {
	// 选项（含费率、展示口径、限流开关等）
	model.InitOptionMap()
	// 渠道与 abilities → 运行快照
	model.InitChannelCache()
	// 定价缓存（含 modelEnableGroups 等）
	model.InvalidatePricingCache()
	// 用户/token 的 Redis 缓存随 AuthVersion 栅栏自然失效；此处无需额外清理。
	if common.MemoryCacheEnabled {
		common.SysLog("runtime state refreshed after database import")
	}
}
