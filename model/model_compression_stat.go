package model

import (
	"sync"

	"github.com/lza6/new-api-Max/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// §压缩统计：按模型累积「出站请求体压缩」的次数与字节，**持久化到 DB**，
// 进程重启不丢（用户明确要求：重启容器不丢统计）。
//
// 写入路径：请求热路径只累加**进程内**计数（无 DB 往返）；后台周期 flush 落库
// （复用 batchUpdate 模式，5s 一次），停机时排空。读取时把「进程内未落库增量 +
// DB 已落库值」相加，保证实时且准确。
type ModelCompressionStat struct {
	ModelName       string `json:"model_name" gorm:"type:varchar(191);primaryKey"`
	Count           int64  `json:"count" gorm:"default:0"`            // 压缩请求数
	OriginalBytes   int64  `json:"original_bytes" gorm:"default:0"`   // 压缩前字节合计
	CompressedBytes int64  `json:"compressed_bytes" gorm:"default:0"` // 压缩后字节合计
	SavedBytes      int64  `json:"saved_bytes" gorm:"default:0"`      // 节省字节 = 原始-压缩后
	UpdatedAt       int64  `json:"updated_at" gorm:"default:0"`
}

func (ModelCompressionStat) TableName() string { return "model_compression_stats" }

// 进程内增量（热路径无锁竞争用分片锁；此处用单锁即可，写入极轻）。
var (
	compressionStatMu     sync.Mutex
	compressionStatBuffer = map[string]*ModelCompressionStat{}
)

// RecordModelCompression 热路径调用：只累加进程内缓冲（无 DB 往返，无阻塞）。
func RecordModelCompression(modelName string, originalBytes, compressedBytes int64) {
	if modelName == "" || originalBytes <= 0 || compressedBytes < 0 {
		return
	}
	saved := originalBytes - compressedBytes
	if saved < 0 {
		saved = 0
	}
	compressionStatMu.Lock()
	e := compressionStatBuffer[modelName]
	if e == nil {
		e = &ModelCompressionStat{ModelName: modelName}
		compressionStatBuffer[modelName] = e
	}
	e.Count++
	e.OriginalBytes += originalBytes
	e.CompressedBytes += compressedBytes
	e.SavedBytes += saved
	compressionStatMu.Unlock()
}

// FlushModelCompressionStats 把进程内缓冲落库（周期 flush / 停机排空调用）。
func FlushModelCompressionStats() {
	compressionStatMu.Lock()
	if len(compressionStatBuffer) == 0 {
		compressionStatMu.Unlock()
		return
	}
	batch := compressionStatBuffer
	compressionStatBuffer = map[string]*ModelCompressionStat{}
	compressionStatMu.Unlock()
	flushModelCompressionStats(batch)
}

func flushModelCompressionStats(batch map[string]*ModelCompressionStat) {
	now := common.GetTimestamp()
	for name, e := range batch {
		row := ModelCompressionStat{
			ModelName:       name,
			Count:           e.Count,
			OriginalBytes:   e.OriginalBytes,
			CompressedBytes: e.CompressedBytes,
			SavedBytes:      e.SavedBytes,
			UpdatedAt:       now,
		}
		// 原子累加（并发/多实例安全）：ON CONFLICT 时对数值列做加法。
		err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "model_name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"count":            gorm.Expr("model_compression_stats.count + ?", row.Count),
				"original_bytes":   gorm.Expr("model_compression_stats.original_bytes + ?", row.OriginalBytes),
				"compressed_bytes": gorm.Expr("model_compression_stats.compressed_bytes + ?", row.CompressedBytes),
				"saved_bytes":      gorm.Expr("model_compression_stats.saved_bytes + ?", row.SavedBytes),
				"updated_at":       now,
			}),
		}).Create(&row).Error
		if err != nil {
			common.SysError("failed to flush model compression stat for " + name + ": " + err.Error())
		}
	}
}

// GetModelCompressionStats 返回按模型压缩统计（DB 已落库 + 进程内未落库增量合并）。
func GetModelCompressionStats() ([]ModelCompressionStat, error) {
	var rows []ModelCompressionStat
	if err := DB.Model(&ModelCompressionStat{}).Order("saved_bytes DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	byName := make(map[string]*ModelCompressionStat, len(rows))
	for i := range rows {
		byName[rows[i].ModelName] = &rows[i]
	}
	compressionStatMu.Lock()
	for name, e := range compressionStatBuffer {
		if cur, ok := byName[name]; ok {
			cur.Count += e.Count
			cur.OriginalBytes += e.OriginalBytes
			cur.CompressedBytes += e.CompressedBytes
			cur.SavedBytes += e.SavedBytes
		} else {
			cp := *e
			rows = append(rows, cp)
			byName[name] = &rows[len(rows)-1]
		}
	}
	compressionStatMu.Unlock()
	return rows, nil
}

// ResetModelCompressionStatBufferForTest 清空进程内缓冲（测试用）。
func ResetModelCompressionStatBufferForTest() {
	compressionStatMu.Lock()
	compressionStatBuffer = map[string]*ModelCompressionStat{}
	compressionStatMu.Unlock()
}
