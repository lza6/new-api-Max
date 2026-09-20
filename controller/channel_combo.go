package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// GetAllCombos 分页列出组合。
func GetAllCombos(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	combos, total, err := model.ListCombos(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(combos)
	common.ApiSuccess(c, pageInfo)
}

// CreateCombo 新建组合。
func CreateCombo(c *gin.Context) {
	combo := model.ChannelCombo{}
	if err := c.ShouldBindJSON(&combo); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.CreateCombo(&combo); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = service.RefreshComboSnapshot() // P2-1 无锁快照失效刷新
	recordManageAudit(c, "combo.create", map[string]any{"name": combo.Name, "strategy": combo.Strategy})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": combo})
}

// UpdateCombo 更新组合。
func UpdateCombo(c *gin.Context) {
	combo := model.ChannelCombo{}
	if err := c.ShouldBindJSON(&combo); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.UpdateCombo(&combo); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = service.RefreshComboSnapshot() // P2-1 无锁快照失效刷新
	recordManageAudit(c, "combo.update", map[string]any{"name": combo.Name})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": combo})
}

// DeleteCombo 删除组合。
func DeleteCombo(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteCombo(id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
