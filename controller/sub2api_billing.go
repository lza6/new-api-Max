package controller

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/setting/ratio_setting"
)

// GetSub2ApiBilling 查询当前密钥所属分组的分组倍率与计费口径（§端点适配，421 契约）。
// GET /v1/sub2api/billing，需 TokenAuth；返回该密钥实际生效的分组倍率，供调用方
// 在提交前预估「基准价 × 分组倍率」的最终扣费口径。
func GetSub2ApiBilling(c *gin.Context) {
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	ratio := ratio_setting.GetGroupRatio(group)
	c.JSON(http.StatusOK, gin.H{
		"object":                    "sub2api.key_billing",
		"schema_version":            1,
		"billing_scope":             "token",
		"group":                     group,
		"group_rate_multiplier":     ratio,
		"resolved_rate_multiplier":  ratio,
		"effective_rate_multiplier": ratio,
		"peak_rate_enabled":         false,
		"observed_at":               time.Now().UTC().Format(time.RFC3339),
	})
}
