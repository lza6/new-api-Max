package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// 训练数据「一键导出」的两张表（增量需求：导出 + 分片下载 + 下载记录）。
//
// ## 为什么是两张新表
//
//   - 现有表都不能承载「导出任务」这个概念：`training_data_consents` 是授权，
//     `training_data_watermarks` 是会话去重水位，二者都与「一次挑选某日期范围
//     打包成一个数据集」无关。
//   - 导出是**用户数据**的管理动作，必须可审计（谁在什么时候导出了什么范围、
//     谁下载过）。下载记录单独成表而不是在导出表上累加一个计数器：
//     计数器无法回答「谁下载的」，而「哪个已经有下载过」正是用户点名的需求。
//
// 两张表都是**纯新增**，不修改任何既有表 —— 回滚只需 DROP TABLE。
//
// ## 三库兼容
//
// 字段只用 int64 / string / bool 与 GORM 支持的 tag；索引用复合 `index:`，
// 不依赖任何 MySQL 8 / PG 15 才有的语法（见 AGENTS.md 数据库兼容要求）。

// TrainingDataExport 一次导出任务。
//
// 生命周期：pending（已创建，待打包）→ building（正在从云盘拉取流式打包）
// → ready（产物就绪，可下载）→ failed（打包失败）。
// 用字符串状态而非 int 枚举：日志/管理端直接可读，且新增状态不需改列类型。
type TrainingDataExport struct {
	ID uint `json:"id" gorm:"primaryKey"`

	// Status 导出状态：pending / building / ready / failed。
	Status string `json:"status" gorm:"type:varchar(16);index:idx_training_export_status"`

	// DateFrom / DateTo 导出涵盖的 UTC 日期范围（含两端，YYYY-MM-DD）。
	// 用字符串而非时间：云盘目录名就是 `YYYY-MM-DD`（见 DateDirName），
	// 按目录名前缀筛选，字符串比较即可，不受部署时区影响。
	DateFrom string `json:"date_from" gorm:"type:varchar(10)"`
	DateTo   string `json:"date_to" gorm:"type:varchar(10)"`

	// UserId / Model 可选过滤条件（0 / 空表示不过滤）。
	// UserId 存**被导出数据的用户 ID**（不是发起导出的管理员）。
	UserId int    `json:"user_id" gorm:"index:idx_training_export_scope"`
	Model  string `json:"model" gorm:"type:varchar(128)"`

	// ArchiveFormat 打包格式（当前仅 "tar"，见 service 层的选择说明）。
	ArchiveFormat string `json:"archive_format" gorm:"type:varchar(16)"`

	// ObjectPath 本地产物路径（相对导出根目录），打包完成后写入。
	// 为空表示产物尚未就绪。
	ObjectPath string `json:"object_path" gorm:"type:varchar(255)"`

	// SizeBytes 产物大小（ready 时为真实值，用于 Content-Length 与展示）。
	SizeBytes int64 `json:"size_bytes"`

	// PartCount / RecordCount 归档中包含的分片数与记录数（尽力统计）。
	PartCount   int `json:"part_count"`
	RecordCount int `json:"record_count"`

	// ErrorMsg 失败原因（failed 时非空）。显式记录而不是只置状态：
	// 管理员要能直接看到「为什么没成」。
	ErrorMsg string `json:"error_msg" gorm:"type:text"`

	// CreatedBy 发起导出的管理员用户 ID。
	CreatedBy int `json:"created_by" gorm:"index:idx_training_export_creator"`

	// DownloadCount 已完成的下载次数（冗余计数，便于列表快速展示；
	// 明细以 TrainingDataExportDownload 为准）。
	DownloadCount int `json:"download_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名（避免 GORM 复数化差异）。
func (TrainingDataExport) TableName() string {
	return "training_data_exports"
}

// 导出状态常量。集中定义，避免各处散落字符串字面量。
const (
	TrainingExportStatusPending  = "pending"
	TrainingExportStatusBuilding = "building"
	TrainingExportStatusReady    = "ready"
	TrainingExportStatusFailed   = "failed"
)

// TrainingDataExportDownload 一次下载记录。
//
// 「哪个导出被谁下载过、何时、是否完成」正是这张表要回答的三件事：
// DownloaderId（谁）、StartedAt（何时）、Completed（是否完成）+ CompletedAt。
// 一条记录在**开始下载**时创建（Completed=false），**下载完成**时回填 ——
// 这样中断的下载也留痕（用户明确要求「记录哪个已经有下载过」）。
type TrainingDataExportDownload struct {
	ID uint `json:"id" gorm:"primaryKey"`

	// ExportId 所属导出。复合索引与下载人一起便于按导出/按人查询。
	ExportId uint `json:"export_id" gorm:"index:idx_training_export_dl,priority:1"`

	// DownloaderId 下载者（管理员用户 ID）。
	DownloaderId int `json:"downloader_id" gorm:"index:idx_training_export_dl,priority:2"`

	// Source 下载方式：browser（浏览器直连）/ direct（直链）/ unknown。
	Source string `json:"source" gorm:"type:varchar(16)"`

	// BytesSent 本次实际下发的字节数（支持续传时是本次区间的大小）。
	BytesSent int64 `json:"bytes_sent"`

	// RangeHeader 客户端携带的 Range 头（空表示全量；用于诊断续传行为）。
	RangeHeader string `json:"range_header" gorm:"type:varchar(128)"`

	// Completed 是否完整下载完成。
	// **诚实边界**：只有响应写完后才判定完成；中断（客户端断开）为 false。
	Completed bool `json:"completed"`

	StartedAt   int64 `json:"started_at"`
	CompletedAt int64 `json:"completed_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名。
func (TrainingDataExportDownload) TableName() string {
	return "training_data_export_downloads"
}

// ---------- 访问方法 ----------

// CreateTrainingDataExport 创建一条导出任务记录（初始 pending）。
func CreateTrainingDataExport(e *TrainingDataExport) error {
	if e.Status == "" {
		e.Status = TrainingExportStatusPending
	}
	return DB.Create(e).Error
}

// GetTrainingDataExport 按 ID 读取导出。
func GetTrainingDataExport(id uint) (*TrainingDataExport, error) {
	var e TrainingDataExport
	err := DB.First(&e, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// ListTrainingDataExports 分页列出导出（最新在前）。
//
// offset/limit 由调用方夹取，避免无界查询。
func ListTrainingDataExports(offset, limit int) ([]TrainingDataExport, int64, error) {
	var total int64
	if err := DB.Model(&TrainingDataExport{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TrainingDataExport
	err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

// UpdateTrainingDataExport 更新导出的可变字段。
//
// 用 map 更新而不是 Save 整个结构体：Save 会把零值一并写回，
// 可能在并发下把别的字段覆盖成零（例如打包中写 ObjectPath 时把 SizeBytes 抹掉）。
func UpdateTrainingDataExport(id uint, updates map[string]any) error {
	return DB.Model(&TrainingDataExport{}).Where("id = ?", id).Updates(updates).Error
}

// ListTrainingDataExportsByStatus 返回指定状态的全部导出（供启动时清理中断的任务）。
func ListTrainingDataExportsByStatus(status string) ([]TrainingDataExport, error) {
	var rows []TrainingDataExport
	err := DB.Where("status = ?", status).Find(&rows).Error
	return rows, err
}

// CreateTrainingDataExportDownload 开始一条下载记录。
func CreateTrainingDataExportDownload(d *TrainingDataExportDownload) error {
	return DB.Create(d).Error
}

// CompleteTrainingDataExportDownload 回填下载完成状态与字节数。
func CompleteTrainingDataExportDownload(id uint, bytesSent int64, completed bool, now int64) error {
	return DB.Model(&TrainingDataExportDownload{}).Where("id = ?", id).Updates(map[string]any{
		"bytes_sent":   bytesSent,
		"completed":    completed,
		"completed_at": now,
	}).Error
}

// IncrementTrainingDataExportDownloadCount 原子自增导出表的下载计数。
//
// 用 go 表达式而非先读后写：并发下载时后者会丢更新。
func IncrementTrainingDataExportDownloadCount(id uint) error {
	return DB.Model(&TrainingDataExport{}).Where("id = ?", id).
		UpdateColumn("download_count", gorm.Expr("download_count + ?", 1)).Error
}

// ListTrainingDataExportDownloads 按导出列出下载记录（最新在前）。
func ListTrainingDataExportDownloads(exportId uint, limit int) ([]TrainingDataExportDownload, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []TrainingDataExportDownload
	err := DB.Where("export_id = ?", exportId).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
