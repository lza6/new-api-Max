/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

type updateModelGroupsRequest struct {
	ModelName string   `json:"model_name"`
	Groups    []string `json:"groups"`
}

// UpdateModelGroups T2 模型分组归类：更新模型的 Groups 字段，并把分组同步到
// 所有含该模型的启用渠道（并集），重建 abilities 后该模型在指定分组可用。
func UpdateModelGroups(c *gin.Context) {
	var req updateModelGroupsRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	req.ModelName = strings.TrimSpace(req.ModelName)
	cleanGroups := cleanModelGroups(req.Groups)
	if req.ModelName == "" || len(cleanGroups) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 1. 更新模型元数据 Groups 字段
	var m model.Model
	if err := model.DB.Where("model_name = ?", req.ModelName).First(&m).Error; err != nil {
		common.ApiErrorMsg(c, "模型不存在")
		return
	}
	m.Groups = strings.Join(cleanGroups, ",")
	if err := m.Update(); err != nil {
		common.ApiError(c, err)
		return
	}

	// 2. 同步到渠道 group 并重建 abilities
	updated, err := service.SyncModelGroupToChannels(req.ModelName, cleanGroups)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"model_name":       req.ModelName,
		"groups":           cleanGroups,
		"updated_channels": updated,
	})
}

func cleanModelGroups(groups []string) []string {
	seen := make(map[string]struct{}, len(groups))
	result := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		result = append(result, g)
	}
	return result
}
