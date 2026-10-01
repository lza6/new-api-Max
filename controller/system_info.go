package controller

import (
	"net/http"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"

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
// 以及当前实时网络上下行（MB/s）与并发水位。供系统信息页「实时请求详情」面板展示。
func GetLiveRequests(c *gin.Context) {
	snap := service.GetLiveRequestsSnapshot()
	inMBps, outMBps := service.GetNetworkThroughput()
	conc := middleware.GetGlobalConcurrencyStats()

	common.ApiSuccess(c, gin.H{
		"active":                snap.Active,
		"finished":              snap.Finished,
		"active_count":          snap.ActiveCount,
		"compressed_count":      snap.CompressedCount,
		"original_bytes_sum":    snap.OriginalBytesSum,
		"compressed_bytes_sum":  snap.CompressedBytesSum,
		"avg_compression_ratio": snap.AvgCompressionRatio,
		"avg_first_response_ms": snap.AvgFirstResponseMs,
		"network_in_mbps":       inMBps,
		"network_out_mbps":      outMBps,
		"concurrency":           conc,
		"compression_enabled":   common.RelayRequestCompressionEnabled,
		"compression_threshold_kb": common.RelayRequestCompressionThresholdKB,
	})
}
