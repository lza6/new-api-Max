package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// G5-3：动作级审批的管理端接口。
//
// 默认拒绝：MCP 的付费 tool 只有在这里被批准后、且未过期，才能执行一次。
// 这些接口挂在 AdminAuth 下（见 router/api-router.go），普通用户不可见。

type actionApprovalReasonRequest struct {
	Reason string `json:"reason"`
}

// GetActionApprovals 分页列出审批记录（status 为空 = 全部）。
func GetActionApprovals(c *gin.Context) {
	// 懒清理过期记录（有界），避免只能靠查询条件兜底、管理端看不到 expired 状态。
	_, _ = service.CleanupActionApprovals()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	if page < 1 {
		page = 1
	}
	rows, total, err := model.ListActionApprovals(c.Query("status"), (page-1)*size, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
		"total":   total,
		"page":    page,
		"size":    size,
	})
}

// ApproveActionApproval 批准一条待审批动作。
func ApproveActionApproval(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid approval id")
		return
	}
	var req actionApprovalReasonRequest
	_ = c.ShouldBindJSON(&req)
	adminId := c.GetInt("id")
	if err := service.ApproveAction(id, adminId, req.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// DenyActionApproval 拒绝一条待审批动作。
func DenyActionApproval(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid approval id")
		return
	}
	var req actionApprovalReasonRequest
	_ = c.ShouldBindJSON(&req)
	adminId := c.GetInt("id")
	if err := service.DenyAction(id, adminId, req.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
