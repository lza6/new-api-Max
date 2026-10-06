package service

import (
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// T1 统一节省口径 + Savings Baseline（litellm 反事实基准迁移）。
//
// 现有压缩统计只报「节省了多少字节/多少 %」。本模块补一个**反事实基准**：
// 回答「如果不做优化，会多花多少」。统一口径为：
//
//	节省率 = saved_bytes / original_bytes（0..1）
//	反事实上传量 = original_bytes（未压缩时本应上传的字节）
//	额外上传量 = saved_bytes（因压缩而免于上传的字节）
//	折算省时 = saved_bytes / 观测上行带宽（bps）→ 秒
//
// 带宽取自 common.OutboundUploadBandwidthBps（env 可配，默认 0=不折算省时）。
// 每模型同时给出次数、原始/压缩/节省字节、节省率，供前端统一展示。

// SavingsBaselineModel 单模型的反事实基准行。
type SavingsBaselineModel struct {
	ModelName       string  `json:"model_name"`
	Count           int64   `json:"count"`
	OriginalBytes   int64   `json:"original_bytes"`
	CompressedBytes int64   `json:"compressed_bytes"`
	SavedBytes      int64   `json:"saved_bytes"`
	SavedRatio      float64 `json:"saved_ratio"`          // saved/original，0..1
	Counterfactual  int64   `json:"counterfactual_bytes"` // = original（未压缩应上传量）
}

// SavingsBaseline 全站反事实基准（含合计）。
type SavingsBaseline struct {
	TotalCount           int64                  `json:"total_count"`
	TotalOriginalBytes   int64                  `json:"total_original_bytes"`
	TotalCompressedBytes int64                  `json:"total_compressed_bytes"`
	TotalSavedBytes      int64                  `json:"total_saved_bytes"`
	OverallSavedRatio    float64                `json:"overall_saved_ratio"`  // 0..1
	CounterfactualBytes  int64                  `json:"counterfactual_bytes"` // 未压缩应上传总量
	SavedTimeSeconds     float64                `json:"saved_time_seconds"`   // 按观测带宽折算
	BandwidthBps         int64                  `json:"bandwidth_bps"`        // 折算所用带宽（0=未配置）
	SavedText            string                 `json:"saved_text"`           // 可读（如 "163.7 MB"）
	Models               []SavingsBaselineModel `json:"models"`
}

// ComputeSavingsBaseline 把按模型的压缩统计聚合为反事实基准（纯函数）。
func ComputeSavingsBaseline(rows []model.ModelCompressionStat) SavingsBaseline {
	b := SavingsBaseline{Models: make([]SavingsBaselineModel, 0, len(rows))}
	for _, r := range rows {
		ratio := 0.0
		if r.OriginalBytes > 0 {
			ratio = float64(r.SavedBytes) / float64(r.OriginalBytes)
		}
		b.TotalCount += r.Count
		b.TotalOriginalBytes += r.OriginalBytes
		b.TotalCompressedBytes += r.CompressedBytes
		b.TotalSavedBytes += r.SavedBytes
		b.Models = append(b.Models, SavingsBaselineModel{
			ModelName:       r.ModelName,
			Count:           r.Count,
			OriginalBytes:   r.OriginalBytes,
			CompressedBytes: r.CompressedBytes,
			SavedBytes:      r.SavedBytes,
			SavedRatio:      ratio,
			Counterfactual:  r.OriginalBytes,
		})
	}
	if b.TotalOriginalBytes > 0 {
		b.OverallSavedRatio = float64(b.TotalSavedBytes) / float64(b.TotalOriginalBytes)
	}
	b.CounterfactualBytes = b.TotalOriginalBytes
	b.BandwidthBps = common.OutboundUploadBandwidthBps
	// 折算省时：saved_bytes * 8 位 / 带宽(bps) = 秒。
	if b.BandwidthBps > 0 {
		bits := float64(b.TotalSavedBytes) * 8.0
		b.SavedTimeSeconds = bits / float64(b.BandwidthBps)
	}
	b.SavedText = common.FormatBytes(b.TotalSavedBytes)
	return b
}
