package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
)

// BatchUserManageRequest 用户批量操作请求。
type BatchUserManageRequest struct {
	Ids    []int  `json:"ids" binding:"required,min=1,max=500,dive,gt=0"`
	Action string `json:"action"` // enable / disable / delete / add_quota
	Mode   string `json:"mode"`   // add_quota 时: add/subtract/override
	Value  int    `json:"value"`  // add_quota 时额度
}

// BatchManageUsers 批量操作用户（启用/禁用/删除/批量加扣额度）。
// 逐用户执行，任何单个失败即中止并返回已处理数量；权限校验同单用户 ManageUser。
func BatchManageUsers(c *gin.Context) {
	var req BatchUserManageRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(req.Ids) > 500 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	myRole := c.GetInt("role")
	processed := 0
	for _, id := range req.Ids {
		user, err := model.GetUserById(id, false)
		if err != nil {
			break
		}
		if !canManageTargetRole(myRole, user.Role) {
			continue // 跳过权限不足的用户，不中断
		}
		switch req.Action {
		case "enable":
			user.Status = common.UserStatusEnabled
			_ = user.Update(false)
		case "disable":
			if user.Role == common.RoleRootUser {
				continue
			}
			user.Status = common.UserStatusDisabled
			_ = user.Update(false)
		case "delete":
			if user.Role == common.RoleRootUser {
				continue
			}
			_ = model.HardDeleteUserById(id)
		case "add_quota":
			_, _ = model.AdjustUserQuota(id, myRole, req.Mode, req.Value)
		default:
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		processed++
	}
	recordManageAudit(c, "user.batch_manage", map[string]any{
		"action":    req.Action,
		"total":     len(req.Ids),
		"processed": processed,
		"user_ids":  req.Ids,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"processed": processed, "total": len(req.Ids)},
	})
}
