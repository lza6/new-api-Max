package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// ResetAllCheckins P1 管理员重置签到：清空全部签到记录，所有用户可重新签到。
// 仅管理员/root 可调用（路由层鉴权）。
func ResetAllCheckins(c *gin.Context) {
	deleted, err := model.ResetAllCheckins()
	if err != nil {
		common.ApiErrorMsg(c, "重置签到失败："+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{
		"deleted": deleted,
		"message": "签到记录已重置，用户可重新签到",
	})
}

// GetCheckinStatus 获取用户签到状态和历史记录
func GetCheckinStatus(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}
	userId := c.GetInt("id")
	// 获取月份参数，默认为当前月份
	month := c.DefaultQuery("month", time.Now().Format("2006-01"))

	stats, err := model.GetUserCheckinStats(userId, month)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":   setting.Enabled,
			"min_quota": setting.MinQuota,
			"max_quota": setting.MaxQuota,
			"stats":     stats,
		},
	})
}

// DoCheckin 执行用户签到
func DoCheckin(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}

	userId := c.GetInt("id")

	checkin, err := model.UserCheckin(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("用户签到，获得额度 %s", logger.LogQuota(checkin.QuotaAwarded)))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "签到成功",
		"data": gin.H{
			"quota_awarded": checkin.QuotaAwarded,
			"checkin_date":  checkin.CheckinDate},
	})
}
