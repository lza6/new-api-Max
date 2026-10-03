package controller

import (
	"net/http"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/relay_setting"

	"github.com/gin-gonic/gin"
)

func ListSystemInstances(c *gin.Context) {
	instances, err := model.ListSystemInstances()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := common.GetTimestamp()
	responses := make([]model.SystemInstanceResponse, 0, len(instances))
	for _, instance := range instances {
		responses = append(responses, instance.ToResponse(now))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responses,
	})
}

func DeleteStaleSystemInstances(c *gin.Context) {
	deletedCount, err := model.DeleteStaleSystemInstances(common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"deleted_count": deletedCount,
	})
}

func DeleteStaleSystemInstance(c *gin.Context) {
	nodeName := c.Param("node_name")
	if strings.TrimSpace(nodeName) == "" {
		common.ApiErrorMsg(c, "node name is required")
		return
	}

	deleted, err := model.DeleteStaleSystemInstance(nodeName, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !deleted {
		common.ApiErrorMsg(c, "instance is not stale or no longer exists")
		return
	}

	common.ApiSuccess(c, gin.H{
		"deleted_count": 1,
	})
}

// GetLiveRequests 返回实时请求详情快照：进行中/最近完成请求、压缩率、平均首字，
// 以及当前实时网络上下行（MB/s）、并发水位与出站压缩累积统计。
// 供系统信息页「实时请求详情」面板展示。
func GetLiveRequests(c *gin.Context) {
	snap := service.GetLiveRequestsSnapshot()
	inMBps, outMBps := service.GetNetworkThroughput()
	conc := middleware.GetGlobalConcurrencyStats()
	totals := relaycommon.GetCompressionTotals()

	common.ApiSuccess(c, gin.H{
		"active":                snap.Active,
		"finished":              snap.Finished,
		"active_count":          snap.ActiveCount,
		"compressed_count":      snap.CompressedCount,
		"original_bytes_sum":    snap.OriginalBytesSum,
		"compressed_bytes_sum":  snap.CompressedBytesSum,
		"avg_compression_ratio": snap.AvgCompressionRatio,
		"avg_first_response_ms": snap.AvgFirstResponseMs,
		"avg_upload_ms":         snap.AvgUploadMs,
		"avg_upstream_ttfb_ms":  snap.AvgUpstreamTtfbMs,
		"avg_compression_ms":    snap.AvgCompressionMs,
		"network_in_mbps":       inMBps,
		"network_out_mbps":      outMBps,
		"concurrency":           conc,
		// 压缩开关/阈值/级别来自管理员热更新配置（relay_setting）；env 作部署级兜底。
		"compression_enabled":      common.RelayRequestCompressionEnabled && relay_setting.GetRequestCompressionEnabled(),
		"compression_threshold_kb": relay_setting.GetRequestCompressionThresholdKB(),
		"compression_level":        relay_setting.GetRequestCompressionLevel(),
		// 出站压缩累积统计（进程内）：累计压缩的字节、节省带宽与压缩总耗时。
		"compression_total_count":            totals.Count,
		"compression_total_original_bytes":   totals.OriginalBytes,
		"compression_total_compressed_bytes": totals.CompressedBytes,
		"compression_total_saved_bytes":      totals.SavedBytes,
		"compression_total_ms":               totals.TotalMs,
		// 4.2.4 中继 gopool worker 可观测：运行中 worker 数与上界（高并发后据此
		// 确认 worker 数被上界约束）。
		"relay_workers":     common.RelayWorkerCount(),
		"relay_workers_max": common.RelayPoolMaxWorkers(),
	})
}
